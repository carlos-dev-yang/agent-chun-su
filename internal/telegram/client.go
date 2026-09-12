package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"chunsu/internal/config"
	"chunsu/internal/secrets"
)

const DefaultAPIBase = "https://api.telegram.org"
const PollSeconds = 20
const HTTPGraceSeconds = 10
const UpdateLimit = 50
const MessageUnits = 4000
const MaxReplyUnits = MessageUnits * 3
const MinSendInterval = time.Second

// APIError deliberately excludes Telegram descriptions and request URLs: both
// can contain private data. Only polling may automatically retry these errors.
type APIError struct {
	Status     int
	Code       int
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Code == http.StatusConflict {
		return "Telegram webhook or concurrent polling conflict; existing receiver configuration was preserved"
	}
	return fmt.Sprintf("Telegram request rejected (HTTP %d, code %d)", e.Status, e.Code)
}
func ErrorCode(err error) string {
	var api *APIError
	if errors.As(err, &api) {
		switch api.Code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "authentication_failed"
		case http.StatusConflict:
			return "poll_conflict"
		}
	}
	return "poll_failed"
}

// RetryDelay backs off failed reads, with a configurable ceiling. Telegram's
// explicit flood-control delay takes precedence over that local ceiling.
func RetryDelay(err error, failures int, limits config.Limits) time.Duration {
	delay := time.Duration(limits.PollSeconds) * time.Second
	ceiling := time.Duration(limits.RetryDelaySeconds) * time.Second
	if ceiling < delay {
		ceiling = delay
	}
	for i := 1; i < failures && delay < ceiling; i++ {
		if delay > ceiling/2 {
			delay = ceiling
		} else {
			delay *= 2
		}
	}
	var api *APIError
	if errors.As(err, &api) && api.RetryAfter > delay {
		delay = api.RetryAfter
	}
	return delay
}

type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username"`
}
type Message struct {
	ID   int64 `json:"message_id"`
	From *User `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text    string          `json:"text"`
	Forward json.RawMessage `json:"forward_origin"`
	ViaBot  json.RawMessage `json:"via_bot"`
}
type Update struct {
	ID      int64    `json:"update_id"`
	Message *Message `json:"message"`
}

func (m *Message) PrivateText() bool {
	return m != nil && m.From != nil && !m.From.IsBot && m.From.ID > 0 && m.Chat.Type == "private" && m.Chat.ID == m.From.ID && m.Text != "" && len(m.Forward) == 0 && len(m.ViaBot) == 0
}

type Client struct {
	base, token string
	http        *http.Client
	limit       int64
	sendMu      sync.Mutex
	nextSend    time.Time
}

func NewClient(base, token string, limits config.Limits) (*Client, error) {
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return nil, errors.New("invalid Telegram API base")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, errors.New("Telegram API requires HTTPS; HTTP is restricted to explicit loopback fixtures")
	}
	parts := strings.Split(token, ":")
	if len(parts) != 2 || len(token) > secrets.MaxSecretBytes || parts[1] == "" {
		return nil, errors.New("invalid bot token format")
	}
	if id, e := strconv.ParseInt(parts[0], 10, 64); e != nil || id <= 0 {
		return nil, errors.New("invalid bot token format")
	}
	for _, r := range parts[1] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return nil, errors.New("invalid bot token format")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{base: strings.TrimRight(base, "/"), token: token, limit: limits.MaxArtifactBytes, http: &http.Client{Transport: transport, Timeout: time.Duration(PollSeconds+HTTPGraceSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) call(ctx context.Context, method string, input, output any) error {
	b, e := json.Marshal(input)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(b))
	if e != nil {
		return errors.New("cannot prepare Telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	response, e := c.http.Do(req)
	if e != nil {
		return errors.Join(errors.New("Telegram request did not return a confirmed result; no automatic replay"), ctx.Err())
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, c.limit+1))
	if e != nil || int64(len(raw)) > c.limit {
		return errors.New("Telegram response unavailable or too large")
	}
	var envelope struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		Code       int             `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return errors.New("invalid Telegram response")
	}
	if response.StatusCode != http.StatusOK || !envelope.OK {
		code := envelope.Code
		if code == 0 {
			code = response.StatusCode
		}
		retry := min(max(envelope.Parameters.RetryAfter, 0), math.MaxInt64/int64(time.Second))
		return &APIError{Status: response.StatusCode, Code: code, RetryAfter: time.Duration(retry) * time.Second}
	}
	if e = json.Unmarshal(envelope.Result, output); e != nil {
		return errors.New("invalid Telegram result")
	}
	return nil
}
func (c *Client) Identity(ctx context.Context) (User, error) {
	var u User
	e := c.call(ctx, "getMe", struct{}{}, &u)
	if e == nil && (!u.IsBot || u.ID <= 0) {
		e = errors.New("Telegram identity is not a bot")
	}
	return u, e
}
func (c *Client) CheckPolling(ctx context.Context) error {
	var r struct {
		URL string `json:"url"`
	}
	if e := c.call(ctx, "getWebhookInfo", struct{}{}, &r); e != nil {
		return e
	}
	if r.URL != "" {
		return &APIError{Status: http.StatusConflict, Code: http.StatusConflict}
	}
	return nil
}
func (c *Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	var r []Update
	e := c.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": PollSeconds, "limit": UpdateLimit, "allowed_updates": []string{"message"}}, &r)
	return r, e
}
func (c *Client) Send(ctx context.Context, chatID int64, text string) ([]int64, error) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if chatID <= 0 {
		return nil, errors.New("a paired private chat is required")
	}
	units := utf16.Encode([]rune(text))
	if len(units) > MaxReplyUnits {
		suffix := utf16.Encode([]rune("\n[답변 길이 한도에 도달했습니다. 이어서 요청해 주세요.]"))
		end := MaxReplyUnits - len(suffix)
		if units[end-1] >= 0xD800 && units[end-1] <= 0xDBFF {
			end--
		}
		units = append(units[:end], suffix...)
	}
	var ids []int64
	for len(units) > 0 {
		if delay := time.Until(c.nextSend); delay > 0 {
			if delay > time.Duration(HTTPGraceSeconds)*time.Second {
				return ids, &APIError{Status: http.StatusTooManyRequests, Code: http.StatusTooManyRequests, RetryAfter: delay}
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ids, ctx.Err()
			case <-timer.C:
			}
		}
		n := min(MessageUnits, len(units))
		if n < len(units) && units[n-1] >= 0xD800 && units[n-1] <= 0xDBFF {
			n--
		}
		var result Message
		e := c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": string(utf16.Decode(units[:n])), "link_preview_options": map[string]bool{"is_disabled": true}}, &result)
		c.nextSend = time.Now().Add(MinSendInterval)
		if e != nil {
			var api *APIError
			if errors.As(e, &api) && api.RetryAfter > MinSendInterval {
				c.nextSend = time.Now().Add(api.RetryAfter)
			}
			return ids, e
		}
		if result.ID <= 0 || result.Chat.ID != chatID {
			return ids, errors.New("Telegram delivery identity was not confirmed")
		}
		ids = append(ids, result.ID)
		units = units[n:]
	}
	return ids, nil
}
