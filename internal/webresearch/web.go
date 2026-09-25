// Package webresearch provides bounded, public, read-only web evidence to the host.
package webresearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"chunsu/internal/audit"
	"chunsu/internal/files"
	"chunsu/internal/platform"

	"golang.org/x/net/html"
)

const braveEndpoint = "https://api.search.brave.com/res/v1/web/search"
const requestTimeout = 18 * time.Second
const maxQueryBytes = 600
const maxURLBytes = 2048
const maxResponseBytes = 256 << 10
const maxTextRunes = 6000
const maxResults = 5
const maxRedirects = 3
const settingsFile = "web.json"
const defaultSearchesPerDay = 20

type Settings struct {
	Enabled        bool `json:"enabled"`
	SearchesPerDay int  `json:"searches_per_day"`
}

func DefaultSettings() Settings { return Settings{SearchesPerDay: defaultSearchesPerDay} }

func LoadSettings(root string) (Settings, error) {
	settings := DefaultSettings()
	dir := filepath.Join(root, "state")
	if err := files.RequirePrivateDir(dir); err != nil {
		return settings, err
	}
	data, err := files.Read(dir, settingsFile, 1024)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil || !json.Valid(data) {
		return settings, errors.New("invalid web settings")
	}
	return settings, settings.Validate()
}

func (settings Settings) Validate() error {
	if settings.SearchesPerDay < 1 || settings.SearchesPerDay > 1000 {
		return errors.New("web searches_per_day must be between 1 and 1000")
	}
	return nil
}

func SaveSettings(root string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(root, "state")
	if err := files.RequirePrivateDir(dir); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if err := audit.Record(root, "web.configuration.prepared", filepath.Join("state", settingsFile), files.Digest(data)); err != nil {
		return err
	}
	return files.Write(dir, settingsFile, data, true)
}

type SearchItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Age         string `json:"age,omitempty"`
}

type SearchResult struct {
	Provider    string       `json:"provider"`
	RetrievedAt time.Time    `json:"retrieved_at"`
	Results     []SearchItem `json:"results"`
}

type Page struct {
	URL         string    `json:"url"`
	Title       string    `json:"title,omitempty"`
	Text        string    `json:"text"`
	Digest      string    `json:"sha256"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Truncated   bool      `json:"truncated"`
}

// KeyRef scopes the credential-store account to one private data home.
func KeyRef(root string) string {
	return "web-" + files.Digest([]byte(filepath.Clean(root)))[:24]
}

func ValidateQuery(query string) error {
	if query != strings.TrimSpace(query) || len(query) == 0 || len(query) > maxQueryBytes || len(strings.Fields(query)) > 75 {
		return errors.New("search query must be nonempty and at most 600 bytes or 75 words")
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return errors.New("search query contains a control character")
		}
	}
	return nil
}

func ValidateURL(raw string) error {
	if raw != strings.TrimSpace(raw) || len(raw) == 0 || len(raw) > maxURLBytes {
		return errors.New("invalid web URL length")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return errors.New("web URL must be public HTTPS without credentials")
	}
	if u.Port() != "" && u.Port() != "443" {
		return errors.New("web URL must use HTTPS port 443")
	}
	if strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), "localhost") {
		return ErrNonPublic
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicIP(ip.Unmap()) {
		return ErrNonPublic
	}
	return nil
}

// PublicError keeps network diagnostics and response bodies out of model history.
func PublicError(err error) string {
	if errors.Is(err, ErrNonPublic) {
		return "공개 인터넷이 아닌 주소는 보안상 열 수 없습니다."
	}
	if errors.Is(err, ErrSearchBudget) {
		return "오늘의 웹 검색 한도에 도달했습니다. 소유자가 한도를 조정하거나 다음 UTC 날짜에 다시 시도할 수 있습니다."
	}
	if errors.Is(err, ErrProviderAuth) {
		return "Brave 검색 API 인증이 거부되었습니다. 호스트에 저장된 키와 구독 상태를 확인해 주세요."
	}
	if errors.Is(err, ErrProviderRate) {
		return "Brave 검색 API가 요청 속도를 제한했습니다. 잠시 후 다시 시도해 주세요."
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "웹 요청이 취소되거나 시간 제한을 넘었습니다."
	}
	return "공개 웹 자료를 가져오지 못했습니다. URL, 접근 제한 또는 호스트 네트워크를 확인해 주세요."
}

var ErrSearchBudget = errors.New("daily web search budget exhausted")
var ErrNonPublic = errors.New("non-public web address")
var ErrProviderAuth = errors.New("search provider authentication rejected")
var ErrProviderRate = errors.New("search provider rate limited")

// ReserveSearch charges an attempt before the provider call. Ambiguous network
// outcomes stay charged, so retries cannot silently bypass the owner's cap.
func ReserveSearch(ctx context.Context, root string, dailyLimit, lockWaitSeconds int) error {
	if dailyLimit < 1 || dailyLimit > 1000 {
		return errors.New("invalid search budget")
	}
	lock, err := platform.Acquire(ctx, root, time.Duration(lockWaitSeconds)*time.Second)
	if err != nil {
		return err
	}
	defer lock.Close()
	dir := filepath.Join(root, "state")
	if err := files.RequirePrivateDir(dir); err != nil {
		return err
	}
	var ledger struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	data, err := files.Read(dir, "web-search-budget.json", 1024)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && json.Unmarshal(data, &ledger) != nil {
		return errors.New("invalid search budget ledger")
	}
	today := time.Now().UTC().Format("2006-01-02")
	if ledger.Day != today {
		ledger.Day, ledger.Count = today, 0
	}
	if ledger.Count >= dailyLimit {
		return ErrSearchBudget
	}
	ledger.Count++
	data, _ = json.Marshal(ledger)
	return files.Write(dir, "web-search-budget.json", data, err == nil)
}

func Search(parent context.Context, query, key string) (SearchResult, error) {
	if err := ValidateQuery(query); err != nil {
		return SearchResult{}, err
	}
	if key == "" {
		return SearchResult{}, errors.New("missing search key")
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	u, _ := url.Parse(braveEndpoint)
	params := u.Query()
	params.Set("q", query)
	params.Set("count", fmt.Sprint(maxResults))
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return SearchResult{}, err
	}
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: requestTimeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return SearchResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return SearchResult{}, ErrProviderAuth
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return SearchResult{}, ErrProviderRate
	}
	if response.StatusCode != http.StatusOK {
		return SearchResult{}, errors.New("search provider returned a non-success status")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(data) > maxResponseBytes {
		return SearchResult{}, errors.New("search response exceeds limit")
	}
	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return SearchResult{}, errors.New("invalid search response")
	}
	result := SearchResult{Provider: "brave", RetrievedAt: time.Now().UTC(), Results: []SearchItem{}}
	for _, item := range payload.Web.Results {
		if len(result.Results) == maxResults {
			break
		}
		if ValidateURL(item.URL) != nil {
			continue
		}
		result.Results = append(result.Results, SearchItem{Title: compact(item.Title, 180), URL: item.URL, Description: compact(item.Description, 350), Age: compact(item.Age, 80)})
	}
	return result, nil
}

func Open(parent context.Context, raw string) (Page, error) {
	if err := ValidateURL(raw); err != nil {
		return Page{}, err
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 32 << 10, DialContext: safeDial}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: requestTimeout, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("too many redirects")
		}
		return ValidateURL(req.URL.String())
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("Accept", "text/html,text/plain;q=0.9")
	req.Header.Set("User-Agent", "Chun-su-public-research/1.0")
	response, err := client.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Page{}, errors.New("page returned a non-success status")
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return Page{}, errors.New("page is not supported text")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Page{}, err
	}
	truncated := len(data) > maxResponseBytes
	if truncated {
		data = data[:maxResponseBytes]
	}
	var title, body string
	if strings.HasPrefix(contentType, "text/html") {
		title, body = htmlText(data)
	} else {
		body = string(data)
	}
	body = strings.ToValidUTF8(body, "")
	body = strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(body) > maxTextRunes {
		runes := []rune(body)
		body = string(runes[:maxTextRunes])
		truncated = true
	}
	if body == "" {
		return Page{}, errors.New("page has no readable text")
	}
	finalURL := response.Request.URL
	finalURL.Fragment = ""
	digest := sha256.Sum256(data)
	return Page{URL: finalURL.String(), Title: compact(title, 200), Text: body, Digest: hex.EncodeToString(digest[:]), RetrievedAt: time.Now().UTC(), Truncated: truncated}, nil
}

func safeDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("only HTTPS port 443 is allowed")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("web address could not be resolved")
	}
	// Reject mixed DNS answers rather than selecting only the benign one.
	for _, address := range addresses {
		ip, ok := netip.AddrFromSlice(address.IP)
		if !ok || !publicIP(ip.Unmap()) {
			return nil, ErrNonPublic
		}
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	var last error
	for _, address := range addresses {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func publicIP(ip netip.Addr) bool {
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() {
		return netip.MustParsePrefix("2000::/3").Contains(ip) && !netip.MustParsePrefix("2001:db8::/32").Contains(ip)
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}

func compact(value string, max int) string {
	value = strings.Join(strings.Fields(strings.ToValidUTF8(value, "")), " ")
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}

func htmlText(data []byte) (string, string) {
	root, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return "", ""
	}
	var title string
	var words []string
	var walk func(*html.Node, bool, bool)
	walk = func(node *html.Node, skipped, inTitle bool) {
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "script" || name == "style" || name == "noscript" || name == "svg" || name == "nav" || name == "footer" || name == "header" {
				skipped = true
			}
			inTitle = name == "title" || inTitle
		}
		if node.Type == html.TextNode && !skipped {
			value := strings.TrimSpace(node.Data)
			if inTitle {
				title += value + " "
			} else if value != "" {
				words = append(words, value)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, skipped, inTitle)
		}
	}
	walk(root, false, false)
	return title, strings.Join(words, " ")
}
