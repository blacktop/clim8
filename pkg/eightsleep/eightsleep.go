package eightsleep

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/log"
)

const (
	defaultClientAPIURL = "https://client-api.8slp.net/v1"
	defaultAppAPIURL    = "https://app-api.8slp.net"
	defaultAuthURL      = "https://auth-api.8slp.net/v1/tokens"

	knownClientID     = "0894c7f33bb94800a03f1f4df13a4f38"
	knownClientSecret = "f0954a3ed5763ba3d06834c73731a32f15f168f47d4f164751275def86db0c76"

	tokenRefreshBufferSec = 120
	defaultTimeoutSec     = 30

	MIN_TEMP_F = 55
	MAX_TEMP_F = 110
	MIN_TEMP_C = 13
	MAX_TEMP_C = 44

	// Retry configuration
	retryMaxAttempts     = 5
	retryInitialInterval = 500 * time.Millisecond
	retryMaxInterval     = 30 * time.Second
	retryMultiplier      = 2.0
	retryJitterFactor    = 0.5 // adds up to 50% random jitter

	maxErrorBodyBytes = 512
)

// HTTPError is returned when the API answers with a non-2xx status.
type HTTPError struct {
	StatusCode int
	Body       string
}

// SubscriptionRequired reports whether the API refused the request with its "subscription
// required" message. The official app can still do some of what the API refuses this way, so
// this describes the refusal rather than proving the feature itself is paid.
func (e *HTTPError) SubscriptionRequired() bool {
	return e.StatusCode == http.StatusForbidden && strings.Contains(e.Body, "subscription required")
}

func (e *HTTPError) Error() string {
	if e.SubscriptionRequired() {
		return `refused by the Eight Sleep API with HTTP 403 "subscription required"`
	}
	status := fmt.Sprintf("HTTP %d %s", e.StatusCode, http.StatusText(e.StatusCode))
	if e.Body == "" {
		return status
	}
	return status + ": " + e.Body
}

// permanentError wraps an error that should not be retried
type permanentError struct{ error }

func (e permanentError) Unwrap() error { return e.error }

// retryAfterError wraps a retryable error for which the server named a minimum wait
type retryAfterError struct {
	error
	wait time.Duration
}

func (e retryAfterError) Unwrap() error { return e.error }

// retryWithBackoff executes fn with exponential backoff and jitter, starting at interval.
// It retries on transient errors but stops immediately on permanent errors or context cancellation.
func retryWithBackoff(ctx context.Context, interval time.Duration, fn func() error) error {
	var lastErr error

	for attempt := range retryMaxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Don't retry permanent errors
		var permErr permanentError
		if errors.As(lastErr, &permErr) {
			return permErr.error
		}

		// Don't sleep after last attempt
		if attempt == retryMaxAttempts-1 {
			break
		}

		// Calculate sleep with jitter: interval * (1 + random[0, jitterFactor])
		jitter := time.Duration(float64(interval) * retryJitterFactor * rand.Float64())
		sleep := interval + jitter

		var retryAfter retryAfterError
		if errors.As(lastErr, &retryAfter) && retryAfter.wait > sleep {
			sleep = min(retryAfter.wait, retryMaxInterval)
		}

		log.Debug("retrying request", "attempt", attempt+1, "sleep", sleep, "err", lastErr)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}

		// Exponential increase, capped at max
		interval = min(time.Duration(float64(interval)*retryMultiplier), retryMaxInterval)
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

type Client struct {
	mu sync.RWMutex

	email, password string
	tz              *time.Location

	clientID, clientSecret string

	clientAPIURL, appAPIURL, authURL string
	retryInterval                    time.Duration

	http  *http.Client
	token *Token

	isPod   bool
	hasBase bool

	me      *Profile
	devices []Device
}

func NewClient(email, password, tz string) (*Client, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone %s: %w", tz, err)
	}

	// Configure transport to prevent HTTP/2 hangs and set reasonable limits
	// Disable HTTP/2 to prevent connection hangs
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2:     false, // Disable HTTP/2 to prevent hangs
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &Client{
		email:         email,
		password:      password,
		tz:            loc,
		clientID:      knownClientID,
		clientSecret:  knownClientSecret,
		clientAPIURL:  defaultClientAPIURL,
		appAPIURL:     defaultAppAPIURL,
		authURL:       defaultAuthURL,
		retryInterval: retryInitialInterval,
		http: &http.Client{
			Timeout:   time.Second * defaultTimeoutSec,
			Transport: transport,
		},
	}, nil
}

/* -------------------- Public high-level API -------------------- */

func (c *Client) Start(ctx context.Context) error {
	if err := c.refreshToken(ctx); err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	if err := c.fetchProfile(ctx); err != nil {
		return fmt.Errorf("failed to fetch profile: %w", err)
	}
	if err := c.RefreshDevices(ctx); err != nil {
		return fmt.Errorf("failed to fetch devices: %w", err)
	}
	return nil
}

func (c *Client) Stop() { /* nothing to close right now */ }

// UserID returns the authenticated user's ID. It is empty until Start succeeds.
func (c *Client) UserID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.me == nil {
		return ""
	}
	return c.me.ID
}

// Info pretty-prints the raw API payloads that back the app's main screens. One failing payload
// does not stop the rest; every failure is reported at the end.
func (c *Client) Info(ctx context.Context) error {
	trendsURL, err := c.trendsURL(c.me.ID, time.Now().AddDate(0, 0, -1), time.Now())
	if err != nil {
		return err
	}
	dumps := []struct{ title, url string }{
		{"TEMPERATURE", c.temperatureURL(c.me.ID)},
		{"AWAY MODE", c.appAPIURL + "/v1/users/" + c.me.ID + "/away-mode"},
		{"TRENDS", trendsURL},
		{"INTERVALS", c.clientAPIURL + "/users/" + c.me.ID + "/intervals"},
		{"ALARMS", c.appAPIURL + "/v2/users/" + c.me.ID + "/alarms"},
		{"HEALTH SURVEY TEST DRIVE", c.appAPIURL + "/v1/health-survey/test-drive"},
		{"SUBSCRIPTIONS", c.appAPIURL + "/v3/users/" + c.me.ID + "/subscriptions"},
		{"AUTOPILOT DETAILS", c.appAPIURL + "/v1/users/" + c.me.ID + "/autopilotDetails"},
	}
	var failures []error
	for _, dump := range dumps {
		var data map[string]any
		if err := c.doJSON(ctx, http.MethodGet, dump.url, nil, &data); err != nil {
			failures = append(failures,
				fmt.Errorf("failed to fetch %s: %w", strings.ToLower(dump.title), err))
			continue
		}
		log.Info(dump.title)
		if err := prettyPrint(data); err != nil {
			return err
		}
	}
	return errors.Join(failures...)
}

func (c *Client) GetReleaseFeatures(ctx context.Context) (map[string]any, error) {
	url := c.appAPIURL + "/v1/users/" + c.me.ID + "/release-features"
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
		return nil, fmt.Errorf("failed to fetch release features: %w", err)
	}
	return data, nil
}

// AudioCategory is one audio category together with the tracks it contains.
type AudioCategory struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Tracks []map[string]any `json:"tracks"`
}

func (c *Client) GetAudioTracks(ctx context.Context) ([]AudioCategory, error) {
	var data struct {
		Categories []AudioCategory `json:"categories"`
	}
	url := c.appAPIURL + "/v1/audio/categories"
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
		return nil, fmt.Errorf("failed to fetch audio categories: %w", err)
	}
	for idx, category := range data.Categories {
		url := c.appAPIURL + "/v1/users/" + c.me.ID + "/audio/tracks?category=" + category.ID
		var tracks struct {
			Tracks []map[string]any `json:"tracks"`
		}
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &tracks); err != nil {
			return nil, fmt.Errorf("failed to fetch audio tracks for %s: %w", category.ID, err)
		}
		data.Categories[idx].Tracks = tracks.Tracks
	}
	return data.Categories, nil
}

// Prime starts a priming cycle on the pod and asks for a completion notification for the user.
func (c *Client) Prime(ctx context.Context) error {
	device, err := c.primaryDevice()
	if err != nil {
		return err
	}
	if device.Priming {
		return fmt.Errorf("device %s is already priming", device.ID)
	}
	url := c.appAPIURL + "/v1/devices/" + device.ID + "/priming/tasks"
	body := map[string]any{
		"notifications": map[string]any{
			"users": []string{c.me.ID},
			"meta":  "rePriming",
		},
	}
	if err := c.doJSON(ctx, http.MethodPost, url, body, nil); err != nil {
		return fmt.Errorf("failed to start priming: %w", err)
	}
	return nil
}

/* -------------------- internal helpers -------------------- */

func (c *Client) headers() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	h.Set("User-Agent", "okhttp/4.9.3")
	h.Set("Connection", "keep-alive")
	c.mu.RLock()
	if c.token != nil {
		h.Set("Authorization", "Bearer "+c.token.Bearer)
	}
	c.mu.RUnlock()
	return h
}

// clearToken invalidates the cached token, forcing re-auth on next request
func (c *Client) clearToken() {
	c.mu.Lock()
	c.token = nil
	c.mu.Unlock()
	log.Debug("token cleared, will re-authenticate on next request")
}

func (c *Client) refreshToken(ctx context.Context) error {
	c.mu.RLock()
	needsRefresh := c.token == nil ||
		time.Until(c.token.Expiration) < time.Second*tokenRefreshBufferSec
	c.mu.RUnlock()
	if !needsRefresh {
		return nil
	}

	body := map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"grant_type":    "password",
		"username":      c.email,
		"password":      c.password,
	}
	var res struct {
		AccessToken string  `json:"access_token"`
		ExpiresIn   float64 `json:"expires_in"`
		UserID      string  `json:"userId"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.authURL, body, &res); err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}
	if res.AccessToken == "" {
		return errors.New("token refresh failed: response has no access_token")
	}

	c.mu.Lock()
	c.token = &Token{
		Bearer:     res.AccessToken,
		Expiration: time.Now().Add(time.Duration(res.ExpiresIn) * time.Second),
		MainID:     res.UserID,
	}
	c.mu.Unlock()
	log.Debug("token refreshed", "expires_in", res.ExpiresIn)
	return nil
}

func (c *Client) fetchProfile(ctx context.Context) error {
	url := c.clientAPIURL + "/users/me"
	var data struct {
		User Profile `json:"user"`
	}
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
		return fmt.Errorf("failed to fetch profile: %w", err)
	}
	c.mu.Lock()
	for _, f := range data.User.Features {
		if f == "cooling" {
			c.isPod = true
		}
		if f == "elevation" {
			c.hasBase = true
		}
	}
	c.me = &data.User
	c.mu.Unlock()
	return nil
}

// RefreshDevices refreshes the cached device list from the API.
// Call this periodically from daemon to keep device cache fresh.
func (c *Client) RefreshDevices(ctx context.Context) error {
	c.mu.RLock()
	deviceIDs := c.me.Devices
	c.mu.RUnlock()

	var newDevices []Device
	for _, deviceID := range deviceIDs {
		reqURL := c.clientAPIURL + "/devices/" + deviceID
		var data struct {
			Result Device `json:"result"`
		}
		if err := c.doJSON(ctx, http.MethodGet, reqURL, nil, &data); err != nil {
			return fmt.Errorf("failed to refresh device %s: %w", deviceID, err)
		}
		newDevices = append(newDevices, data.Result)
	}

	c.mu.Lock()
	c.devices = newDevices
	c.mu.Unlock()

	log.Debug("device cache refreshed", "count", len(newDevices))
	return nil
}

// primaryDevice returns the first pod on the account.
func (c *Client) primaryDevice() (Device, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.devices) == 0 {
		return Device{}, errors.New("no Eight Sleep devices found on this account")
	}
	return c.devices[0], nil
}

// send performs a single HTTP attempt and classifies the failure for retryWithBackoff.
func (c *Client) send(ctx context.Context, method, reqURL string, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, permanentError{fmt.Errorf("failed to create request: %w", err)}
	}
	req.Header = c.headers()

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s request: %w", method, err)
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if res.StatusCode < 300 {
		return data, nil
	}

	httpErr := &HTTPError{StatusCode: res.StatusCode}
	// Auth responses can echo credentials, so their bodies never reach errors or logs.
	if reqURL != c.authURL {
		httpErr.Body = strings.TrimSpace(string(data[:min(len(data), maxErrorBodyBytes)]))
	}
	switch {
	case res.StatusCode == http.StatusTooManyRequests:
		seconds, _ := strconv.Atoi(res.Header.Get("Retry-After"))
		return nil, retryAfterError{httpErr, time.Duration(seconds) * time.Second}
	case res.StatusCode >= 400 && res.StatusCode < 500:
		return nil, permanentError{httpErr}
	}
	return nil, httpErr
}

// doJSON sends an authenticated JSON request and decodes the response into out.
// A nil out discards the response body.
func (c *Client) doJSON(ctx context.Context, method, reqURL string, payload any, out any) error {
	var payloadBytes []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		payloadBytes = b
	}

	isAuth := reqURL == c.authURL
	if !isAuth {
		if err := c.refreshToken(ctx); err != nil {
			return err
		}
	}

	var data []byte
	doRequest := func() error {
		return retryWithBackoff(ctx, c.retryInterval, func() error {
			var err error
			data, err = c.send(ctx, method, reqURL, payloadBytes)
			return err
		})
	}

	err := doRequest()

	// Handle 401 with a single re-auth (skip for auth endpoint to avoid infinite loop)
	var httpErr *HTTPError
	if !isAuth && errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
		log.Info("received 401, clearing token and re-authenticating")
		c.clearToken()
		if refreshErr := c.refreshToken(ctx); refreshErr != nil {
			return fmt.Errorf("re-auth failed after 401: %w", refreshErr)
		}
		err = doRequest()
	}
	if err != nil {
		return err
	}

	if isAuth {
		log.Debugf("HTTP %s %s (body withheld)", method, reqURL)
	} else {
		log.Debugf("HTTP %s %s\n%s", method, reqURL, string(data))
	}

	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("failed to decode %s %s response: %w", method, reqURL, err)
	}
	return nil
}

func prettyPrint(data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}
	err = quick.Highlight(os.Stdout, string(jsonData)+"\n", "json", "terminal256", "nord")
	if err != nil {
		return fmt.Errorf("failed to highlight json: %v", err)
	}
	return nil
}
