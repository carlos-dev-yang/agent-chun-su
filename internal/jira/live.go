package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chunsu/internal/config"
)

const atlassianAPIOrigin = "https://api.atlassian.com"
const liveAttempts = 3

type LiveAPIError struct {
	Kind   string `json:"kind"`
	Status int    `json:"http_status,omitempty"`
}

func (e *LiveAPIError) Error() string {
	if e.Status == 0 {
		return "Jira " + e.Kind
	}
	return fmt.Sprintf("Jira %s (HTTP %d)", e.Kind, e.Status)
}

type LiveClient struct {
	profile  Profile
	token    string
	asOfDate string
	http     *http.Client
	limits   config.Limits
}

func NewLiveClient(profile Profile, token string, limits config.Limits) (*LiveClient, error) {
	if err := profile.Validate(config.Config{Version: config.Version, Limits: limits, Timezone: profile.Timezone}); err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("Jira credential is empty")
	}
	return &LiveClient{profile: profile, token: token, limits: limits, http: &http.Client{Timeout: time.Duration(config.DefaultTimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Jira redirects are not permitted") }}}, nil
}
func (c *LiveClient) endpoint(parts ...string) (*url.URL, error) {
	u, err := url.Parse(atlassianAPIOrigin)
	if err != nil {
		return nil, err
	}
	segments := append([]string{"ex", "jira", c.profile.CloudID}, parts...)
	p, err := url.JoinPath(u.String(), segments...)
	if err != nil {
		return nil, errors.New("invalid fixed Jira route")
	}
	u, err = url.Parse(p)
	if err != nil || u.Scheme != "https" || u.Host != "api.atlassian.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid fixed Jira origin")
	}
	return u, nil
}
func apiFailure(status int) *LiveAPIError {
	e := &LiveAPIError{Kind: "provider_error", Status: status}
	switch status {
	case http.StatusUnauthorized:
		e.Kind = "waiting_auth"
	case http.StatusForbidden:
		e.Kind = "permission_denied"
	case http.StatusTooManyRequests:
		e.Kind = "rate_limited"
	case http.StatusNotFound:
		e.Kind = "not_found"
	case http.StatusBadRequest:
		e.Kind = "invalid_request_or_cursor"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		e.Kind = "transient_provider"
	}
	return e
}
func transient(kind string) bool {
	return kind == "rate_limited" || kind == "transient_provider" || kind == "transient_network"
}
func (c *LiveClient) get(ctx context.Context, parts []string, query url.Values) ([]byte, error) {
	u, err := c.endpoint(parts...)
	if err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()
	for attempt := 0; attempt < liveAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, errors.New("cannot construct Jira GET request")
		}
		// This connector accepts only a user-owned personal API token. Atlassian
		// Cloud's REST gateway authenticates that token as Basic email:token;
		// OAuth bearer credentials require a separate reviewed profile type.
		req.SetBasicAuth(c.profile.ExpectedAccount, c.token)
		req.Header.Set("Accept", "application/json")
		response, err := c.http.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, c.limits.MaxArtifactBytes+1))
			response.Body.Close()
			if readErr != nil || int64(len(data)) > c.limits.MaxArtifactBytes {
				return nil, &LiveAPIError{Kind: "response_unavailable_or_oversized", Status: response.StatusCode}
			}
			if response.StatusCode == http.StatusOK {
				return data, nil
			}
			failure := apiFailure(response.StatusCode)
			if !transient(failure.Kind) || attempt+1 == liveAttempts {
				return nil, failure
			}
		} else {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt+1 == liveAttempts {
				return nil, &LiveAPIError{Kind: "transient_network"}
			}
		}
		wait := time.NewTimer(time.Duration(1<<attempt) * time.Second)
		select {
		case <-ctx.Done():
			wait.Stop()
			return nil, ctx.Err()
		case <-wait.C:
		}
	}
	return nil, &LiveAPIError{Kind: "transient_network"}
}

type verifiedIdentity struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
}

func (c *LiveClient) Myself(ctx context.Context) (verifiedIdentity, error) {
	var out verifiedIdentity
	data, err := c.get(ctx, []string{"rest", "api", "3", "myself"}, nil)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(data, &out) != nil {
		return out, errors.New("invalid Jira identity response")
	}
	return out, nil
}

type verifiedBoard struct{ ProjectKey string }

func (c *LiveClient) Board(ctx context.Context, id, todoStatusID, todoStatusName string) (verifiedBoard, error) {
	var metadata struct {
		Location struct {
			ProjectKey string `json:"projectKey"`
		} `json:"location"`
	}
	data, err := c.get(ctx, []string{"rest", "agile", "1.0", "board", id}, nil)
	if err != nil {
		return verifiedBoard{}, err
	}
	if json.Unmarshal(data, &metadata) != nil || !projectKey(metadata.Location.ProjectKey) {
		return verifiedBoard{}, errors.New("invalid Jira board metadata response")
	}
	var configuration struct {
		ColumnConfig struct {
			Columns []struct {
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	data, err = c.get(ctx, []string{"rest", "agile", "1.0", "board", id, "configuration"}, nil)
	if err != nil {
		return verifiedBoard{}, err
	}
	if json.Unmarshal(data, &configuration) != nil || len(configuration.ColumnConfig.Columns) == 0 {
		return verifiedBoard{}, errors.New("invalid Jira board column configuration response")
	}
	todoColumns := 0
	for _, column := range configuration.ColumnConfig.Columns {
		for _, status := range column.Statuses {
			if status.ID == todoStatusID {
				todoColumns++
			}
		}
	}
	if todoColumns != 1 {
		return verifiedBoard{}, errors.New("configured Jira TODO status must occur in exactly one reviewed board column")
	}
	var status struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	data, err = c.get(ctx, []string{"rest", "api", "3", "status", todoStatusID}, nil)
	if err != nil {
		return verifiedBoard{}, err
	}
	if json.Unmarshal(data, &status) != nil || status.ID != todoStatusID || status.Name != todoStatusName || status.StatusCategory.Key != "new" {
		return verifiedBoard{}, errors.New("configured Jira TODO status does not match its verified status metadata")
	}
	return verifiedBoard{ProjectKey: metadata.Location.ProjectKey}, nil
}
func (c *LiveClient) Fields(ctx context.Context, required ...string) error {
	var fields []struct {
		ID string `json:"id"`
	}
	data, err := c.get(ctx, []string{"rest", "api", "3", "field"}, nil)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, &fields) != nil {
		return errors.New("invalid Jira field response")
	}
	found := map[string]bool{}
	for _, field := range fields {
		found[field.ID] = true
	}
	for _, id := range required {
		if !found[id] {
			return fmt.Errorf("reviewed Jira field %s is not available", id)
		}
	}
	return nil
}
func quoteJQL(value string) string { return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\"" }
func (c *LiveClient) IssuePage(ctx context.Context, policy Policy, cursor string) (Page, error) {
	if policy.ConnectionID != c.profile.ID || policy.Subject.ID != c.profile.SubjectAccountID || len(policy.ProjectKeys) != 1 || policy.ProjectKeys[0] != c.profile.ProjectKey {
		return Page{}, errors.New("live Jira reader policy/profile mismatch")
	}
	q := url.Values{}
	q.Set("maxResults", fmt.Sprintf("%d", policy.MaxIssues))
	asOf, err := time.Parse(time.DateOnly, c.asOfDate)
	if err != nil {
		return Page{}, errors.New("invalid pinned Jira as-of date")
	}
	through := asOf.AddDate(0, 0, c.profile.Policy.WindowDays).Format(time.DateOnly)
	q.Set("jql", "project = "+quoteJQL(c.profile.ProjectKey)+" AND assignee = "+quoteJQL(c.profile.SubjectAccountID)+" AND statusCategory != Done AND (duedate <= "+quoteJQL(through)+" OR (duedate IS EMPTY AND status = "+quoteJQL(c.profile.TodoStatusID)+"))")
	fields := []string{"summary", "status", "assignee", "project", "duedate", c.profile.StartField, "created", "updated", "resolutiondate", "resolution"}
	if c.profile.Policy.ContentScope == "metadata_and_description" {
		fields = append(fields, "description")
	}
	q.Set("fields", strings.Join(fields, ","))
	if cursor != "" {
		q.Set("nextPageToken", cursor)
	}
	data, err := c.get(ctx, []string{"rest", "software", "1.0", "board", c.profile.BoardID, "issue"}, q)
	if err != nil {
		return Page{}, err
	}
	next, complete, err := pageContinuation(data, CloudV3)
	if err != nil {
		return Page{}, err
	}
	return Page{Body: data, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), NextCursor: next, Complete: complete}, nil
}

type LiveReader struct{ client *LiveClient }

func NewLiveReader(profile Profile, token, asOfDate string, limits config.Limits) (*LiveReader, error) {
	if _, err := time.Parse(time.DateOnly, asOfDate); err != nil {
		return nil, errors.New("Jira reader requires a pinned as-of date")
	}
	client, err := NewLiveClient(profile, token, limits)
	if err != nil {
		return nil, err
	}
	client.asOfDate = asOfDate
	return &LiveReader{client: client}, nil
}
func (r *LiveReader) ReadPage(ctx context.Context, request Request) (Page, error) {
	return r.client.IssuePage(ctx, request.Policy, request.Cursor)
}
