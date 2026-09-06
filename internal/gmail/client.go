package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"chunsu/internal/config"
	"golang.org/x/oauth2"
)

const currentUser = "me"
const MaxProviderIDLength = 256
const retryBase = time.Second

type APIError struct {
	Kind              string `json:"kind"`
	Status            int    `json:"http_status,omitempty"`
	RetryAfterSeconds int64  `json:"retry_after_seconds,omitempty"`
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return "Gmail " + e.Kind
	}
	return fmt.Sprintf("Gmail %s (HTTP %d)", e.Kind, e.Status)
}

type Client struct {
	endpoints Endpoints
	http      *http.Client
	limits    config.Limits
}

func newClient(ctx context.Context, endpoints Endpoints, source oauth2.TokenSource, limits config.Limits) *Client {
	client := oauth2.NewClient(ctx, source)
	base := plainHTTP()
	client.Timeout = base.Timeout
	client.CheckRedirect = base.CheckRedirect
	return &Client{endpoints: endpoints, http: client, limits: limits}
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Body struct {
	AttachmentID string `json:"attachmentId"`
	Size         int64  `json:"size"`
	Data         string `json:"data"`
}
type Part struct {
	PartID   string   `json:"partId"`
	MIME     string   `json:"mimeType"`
	Filename string   `json:"filename"`
	Headers  []Header `json:"headers"`
	Body     Body     `json:"body"`
	Parts    []Part   `json:"parts"`
}
type APIMessage struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"threadId"`
	LabelIDs     []string `json:"labelIds"`
	InternalDate string   `json:"internalDate"`
	Payload      Part     `json:"payload"`
}
type MessageID struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}
type Page struct {
	Messages      []MessageID `json:"messages"`
	NextPageToken string      `json:"nextPageToken"`
}

func providerID(id string) bool {
	if len(id) == 0 || len(id) > MaxProviderIDLength {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(time.Until(at), 0)
	}
	return retryBase * time.Duration(1<<attempt)
}
func (c *Client) get(ctx context.Context, segments []string, query url.Values, target any) error {
	endpoint, err := url.JoinPath(c.endpoints.API, append([]string{"users", currentUser}, segments...)...)
	if err != nil {
		return errors.New("invalid Gmail route")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("invalid Gmail endpoint")
	}
	u.RawQuery = query.Encode()
	for attempt := 0; attempt < DefaultHTTPAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return errors.New("cannot construct Gmail read")
		}
		response, err := c.http.Do(req)
		var failure *APIError
		var delay time.Duration
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.As(err, &failure) {
				return failure
			}
			failure = &APIError{Kind: "transient_network"}
			delay = retryDelay("", attempt)
		} else {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, c.limits.MaxArtifactBytes+1))
			response.Body.Close()
			if readErr != nil || int64(len(data)) > c.limits.MaxArtifactBytes {
				return &APIError{Kind: "response_unavailable_or_oversized", Status: response.StatusCode}
			}
			if response.StatusCode == http.StatusOK {
				if err = json.Unmarshal(data, target); err != nil {
					return &APIError{Kind: "invalid_response"}
				}
				return nil
			}
			failure = &APIError{Kind: "provider_error", Status: response.StatusCode}
			switch response.StatusCode {
			case http.StatusUnauthorized:
				failure.Kind = "waiting_auth"
			case http.StatusForbidden:
				failure.Kind = "permission_denied"
				var detail struct {
					Error struct {
						Errors []struct {
							Reason string `json:"reason"`
						} `json:"errors"`
					} `json:"error"`
				}
				_ = json.Unmarshal(data, &detail)
				for _, e := range detail.Error.Errors {
					if e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" || e.Reason == "dailyLimitExceeded" {
						failure.Kind = "rate_limited"
					}
				}
			case http.StatusTooManyRequests:
				failure.Kind = "rate_limited"
			case http.StatusNotFound:
				failure.Kind = "not_found"
			case http.StatusBadRequest:
				failure.Kind = "invalid_request_or_cursor"
			case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
				failure.Kind = "transient_provider"
			}
			delay = retryDelay(response.Header.Get("Retry-After"), attempt)
		}
		transient := strings.HasPrefix(failure.Kind, "transient_") || failure.Kind == "rate_limited"
		failure.RetryAfterSeconds = int64(delay / time.Second)
		if !transient || attempt == DefaultHTTPAttempts-1 || delay > time.Duration(DefaultMaxRetrySeconds)*time.Second {
			return failure
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return &APIError{Kind: "retry_budget_exhausted"}
}
func (c *Client) Profile(ctx context.Context) (string, error) {
	var profile struct {
		Email string `json:"emailAddress"`
	}
	err := c.get(ctx, []string{"profile"}, url.Values{"fields": []string{"emailAddress"}}, &profile)
	if err != nil {
		return "", err
	}
	if profile.Email == "" {
		return "", &APIError{Kind: "missing_account_identity"}
	}
	return profile.Email, nil
}
func (c *Client) List(ctx context.Context, query, token string, maxResults int) (Page, error) {
	var page Page
	if maxResults <= 0 || maxResults > ProviderMaxPage {
		return page, errors.New("invalid Gmail page limit")
	}
	q := url.Values{"q": []string{query}, "maxResults": []string{strconv.Itoa(maxResults)}, "includeSpamTrash": []string{"false"}, "fields": []string{"messages(id,threadId),nextPageToken"}}
	if token != "" {
		q.Set("pageToken", token)
	}
	err := c.get(ctx, []string{"messages"}, q, &page)
	return page, err
}
func (c *Client) Message(ctx context.Context, id string) (APIMessage, error) {
	var m APIMessage
	if !providerID(id) {
		return m, errors.New("invalid Gmail message identity")
	}
	err := c.get(ctx, []string{"messages", id}, url.Values{"format": []string{"full"}, "fields": []string{"id,threadId,labelIds,internalDate,payload"}}, &m)
	if err == nil && (m.ID != id || !providerID(m.ThreadID)) {
		return m, errors.New("Gmail returned an inconsistent message identity")
	}
	return m, err
}
func (c *Client) Thread(ctx context.Context, id string) ([]APIMessage, error) {
	var thread struct {
		ID       string       `json:"id"`
		Messages []APIMessage `json:"messages"`
	}
	if !providerID(id) {
		return nil, errors.New("invalid Gmail thread identity")
	}
	err := c.get(ctx, []string{"threads", id}, url.Values{"format": []string{"full"}, "fields": []string{"id,messages(id,threadId,labelIds,internalDate,payload)"}}, &thread)
	if err == nil && thread.ID != id {
		return nil, errors.New("Gmail returned an inconsistent thread identity")
	}
	return thread.Messages, err
}
