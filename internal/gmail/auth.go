package gmail

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"chunsu/internal/config"
	"chunsu/internal/files"
	"chunsu/internal/secrets"
	"golang.org/x/oauth2"
)

type desktopCredentials struct {
	Installed *struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
}
type callbackResult struct {
	code    string
	failure string
}

func plainHTTP() *http.Client {
	return &http.Client{Timeout: time.Duration(DefaultHTTPTimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirects are not permitted") }}
}
func oauthConfig(c Connection, secret, redirect string) *oauth2.Config {
	return &oauth2.Config{ClientID: c.ClientID, ClientSecret: secret, Endpoint: oauth2.Endpoint{AuthURL: c.Endpoints.Authorization, TokenURL: c.Endpoints.Token, AuthStyle: oauth2.AuthStyleInParams}, RedirectURL: redirect, Scopes: []string{ReadOnlyScope}}
}

func Connect(parent context.Context, root string, c config.Config, clientData []byte, expectedAccount string, policy Policy, previous *Connection, present func(string) error) (Connection, error) {
	var connection Connection
	if err := policy.Validate(c); err != nil {
		return connection, err
	}
	if err := CheckAccount(expectedAccount, expectedAccount); err != nil {
		return connection, err
	}
	var credentials desktopCredentials
	if err := json.Unmarshal(clientData, &credentials); err != nil {
		return connection, errors.New("OAuth client file is not valid JSON")
	}
	if credentials.Installed == nil || credentials.Installed.ClientID == "" {
		return connection, errors.New("provide a Google OAuth Desktop app client JSON file")
	}
	if len(credentials.Installed.ClientSecret) > secrets.MaxSecretBytes {
		return connection, errors.New("OAuth client secret exceeds the supported size")
	}
	k, err := secrets.Open()
	if err != nil {
		return connection, err
	}
	preflight, finishPreflight := context.WithTimeout(parent, time.Duration(DefaultHTTPTimeoutSeconds)*time.Second)
	markerRef, marker := files.ID(), files.ID()
	err = k.Set(preflight, markerRef, marker)
	cleanupErr := deleteReferences(parent, k, markerRef)
	finishPreflight()
	if err != nil || cleanupErr != nil {
		return connection, errors.Join(err, cleanupErr)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(DefaultAuthTimeoutSeconds)*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", net.JoinHostPort(LoopbackHost, "0"))
	if err != nil {
		return connection, errors.New("cannot open local OAuth callback")
	}
	callbackHost := listener.Addr().String()
	redirect := "http://" + callbackHost + "/"
	connection = Connection{Version: Version, ID: files.ID(), ClientID: credentials.Installed.ClientID, ClientSecretRef: files.ID(), RefreshRef: files.ID(), Policy: policy, Endpoints: DefaultEndpoints(), Enabled: true}
	if previous != nil {
		if err = CheckAccount(previous.Account, expectedAccount); err != nil {
			return Connection{}, err
		}
		if PolicyDigest(previous.Policy) != PolicyDigest(policy) {
			return Connection{}, errors.New("reauthorization cannot silently expand the existing policy")
		}
		connection.ID = previous.ID
	}
	flow := oauthConfig(connection, credentials.Installed.ClientSecret, redirect)
	verifier := oauth2.GenerateVerifier()
	state := files.ID()
	results := make(chan callbackResult, 1)
	var once sync.Once
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		if r.Method != http.MethodGet || r.Host != callbackHost || r.URL.Path != "/" {
			http.Error(w, "Invalid callback", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid authorization state", http.StatusBadRequest)
			return
		}
		code, failure := r.URL.Query().Get("code"), r.URL.Query().Get("error")
		if code == "" && failure == "" {
			http.Error(w, "Missing authorization result", http.StatusBadRequest)
			return
		}
		once.Do(func() { results <- callbackResult{code: code, failure: failure} })
		_, _ = io.WriteString(w, "Authorization response received. Return to the terminal to check the result.")
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Duration(DefaultHTTPTimeoutSeconds) * time.Second, WriteTimeout: time.Duration(DefaultHTTPTimeoutSeconds) * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	authURL := flow.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "consent"), oauth2.SetAuthURLParam("login_hint", expectedAccount))
	if err = present(authURL); err != nil {
		return Connection{}, err
	}
	var received callbackResult
	select {
	case <-ctx.Done():
		return Connection{}, errors.New("Gmail authorization did not complete before the timeout")
	case received = <-results:
	}
	if received.failure != "" {
		return Connection{}, errors.New("Gmail authorization was declined or blocked; no account was saved")
	}
	httpCtx := context.WithValue(ctx, oauth2.HTTPClient, plainHTTP())
	token, err := flow.Exchange(httpCtx, received.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Connection{}, errors.New("OAuth code exchange failed; check the desktop client and retry authorization")
	}
	scopes, _ := token.Extra("scope").(string)
	connection.Scopes = strings.Fields(scopes)
	if err = CheckScopes(connection.Scopes); err != nil {
		return Connection{}, err
	}
	if token.RefreshToken == "" || len(token.RefreshToken) > secrets.MaxSecretBytes {
		return Connection{}, errors.New("Google did not provide a supported offline refresh token; authorize the desktop client again")
	}
	client := newClient(httpCtx, connection.Endpoints, oauth2.StaticTokenSource(token), c.Limits)
	account, err := client.Profile(httpCtx)
	if err != nil {
		return Connection{}, err
	}
	if err = CheckAccount(expectedAccount, account); err != nil {
		return Connection{}, err
	}
	connection.Account = account
	connection.ConnectedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if previous != nil {
		connection.RetiredSecretRefs = append(append([]string{}, previous.RetiredSecretRefs...), previous.RefreshRef, previous.ClientSecretRef)
	}
	// Separate records avoid native Keychain command-size limits. Access tokens
	// remain in memory and are never copied into configuration or execution data.
	clientSecret := credentials.Installed.ClientSecret
	if clientSecret == "" {
		return Connection{}, errors.New("this desktop client file has no client secret; the current connector requires the downloaded Desktop app credentials")
	}
	if err = k.Set(ctx, connection.ClientSecretRef, clientSecret); err != nil {
		return Connection{}, err
	}
	if err = k.Set(ctx, connection.RefreshRef, token.RefreshToken); err != nil {
		_ = deleteReferences(ctx, k, connection.RefreshRef, connection.ClientSecretRef)
		return Connection{}, err
	}
	if err = SaveConnection(root, connection, previous != nil); err != nil {
		_ = deleteReferences(ctx, k, connection.RefreshRef, connection.ClientSecretRef)
		return Connection{}, err
	}
	if previous != nil {
		if err = deleteReferences(ctx, k, connection.RetiredSecretRefs...); err != nil {
			return connection, errors.New("new authorization is saved; previous Keychain references still need cleanup")
		}
		connection.RetiredSecretRefs = nil
		if err = SaveConnection(root, connection, true); err != nil {
			return connection, err
		}
	}
	return connection, nil
}

type persistTokenSource struct {
	inner    oauth2.TokenSource
	keychain secrets.Keychain
	ref      string
	previous string
	ctx      context.Context
	mu       sync.Mutex
}

func (s *persistTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.inner.Token()
	if err != nil {
		return nil, &APIError{Kind: "waiting_auth"}
	}
	if token.RefreshToken != "" && token.RefreshToken != s.previous {
		if err = s.keychain.Set(s.ctx, s.ref, token.RefreshToken); err != nil {
			return nil, &APIError{Kind: "waiting_auth"}
		}
		s.previous = token.RefreshToken
	}
	return token, nil
}

func OpenClient(ctx context.Context, connection Connection, c config.Config) (*Client, error) {
	if !connection.Enabled {
		return nil, errors.New("connection is disabled")
	}
	if err := connectionEndpoints(connection); err != nil {
		return nil, err
	}
	k, err := secrets.Open()
	if err != nil {
		return nil, err
	}
	clientSecret, err := k.Get(ctx, connection.ClientSecretRef)
	if err != nil {
		return nil, err
	}
	refresh, err := k.Get(ctx, connection.RefreshRef)
	if err != nil {
		return nil, err
	}
	httpCtx := context.WithValue(ctx, oauth2.HTTPClient, plainHTTP())
	flow := oauthConfig(connection, clientSecret, "")
	source := &persistTokenSource{inner: flow.TokenSource(httpCtx, &oauth2.Token{RefreshToken: refresh}), keychain: k, ref: connection.RefreshRef, previous: refresh, ctx: ctx}
	return newClient(httpCtx, connection.Endpoints, source, c.Limits), nil
}
func connectionEndpoints(c Connection) error {
	if err := CheckScopes(c.Scopes); err != nil {
		return err
	}
	return c.Endpoints.Validate()
}

func Disconnect(ctx context.Context, root string, connection Connection, revoke bool) error {
	connection.Enabled = false
	if err := SaveConnection(root, connection, true); err != nil {
		return err
	}
	k, err := secrets.Open()
	if err != nil {
		return err
	}
	if revoke {
		token, e := k.Get(ctx, connection.RefreshRef)
		if e != nil {
			return e
		}
		body := url.Values{"token": []string{token}}
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, connection.Endpoints.Revoke, strings.NewReader(body.Encode()))
		if e != nil {
			return errors.New("invalid revocation request")
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response, e := plainHTTP().Do(req)
		if e != nil {
			return errors.New("remote token revocation could not be confirmed; local connection is disabled")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return errors.New("remote token revocation could not be confirmed; local connection is disabled")
		}
	}
	return deleteReferences(ctx, k, append(append([]string{}, connection.RetiredSecretRefs...), connection.RefreshRef, connection.ClientSecretRef)...)
}

func deleteReferences(parent context.Context, k secrets.Keychain, refs ...string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), time.Duration(DefaultHTTPTimeoutSeconds)*time.Second)
	defer cancel()
	var combined error
	for _, ref := range refs {
		combined = errors.Join(combined, k.Delete(ctx, ref))
	}
	return combined
}
