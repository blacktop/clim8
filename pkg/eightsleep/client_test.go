package eightsleep

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testMe      = "user-me"
	testPartner = "user-partner"
	testDevice  = "device-1"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   map[string]any
}

// fakeAPI answers for all three Eight Sleep hosts in-process and records every request.
type fakeAPI struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fakeResponse struct {
	status  int
	body    string
	headers map[string]string
}

func ok(body string) fakeResponse { return fakeResponse{status: http.StatusOK, body: body} }

func (f *fakeAPI) calls(method, path string) []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var matched []recordedRequest
	for _, r := range f.requests {
		if r.Method == method && r.Path == path {
			matched = append(matched, r)
		}
	}
	return matched
}

// newTestClient returns a client that is already authenticated as testMe on a two-sided pod.
func newTestClient(t *testing.T, respond func(recordedRequest) fakeResponse) (*Client, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rec := recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
		}
		if r.Body != nil {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &rec.Body); err != nil {
					t.Errorf("request body is not JSON: %v", err)
				}
			}
		}
		api.mu.Lock()
		api.requests = append(api.requests, rec)
		api.mu.Unlock()

		resp := respond(rec)
		header := http.Header{}
		for key, value := range resp.headers {
			header.Set(key, value)
		}
		return &http.Response{
			StatusCode: resp.status,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(resp.body)),
			Request:    r,
		}, nil
	})

	c, err := NewClient("sleeper@example.com", "hunter2", "UTC")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.http = &http.Client{Transport: transport}
	c.clientAPIURL = "https://eight.test/client"
	c.appAPIURL = "https://eight.test/app"
	c.authURL = "https://eight.test/auth/tokens"
	c.retryInterval = time.Millisecond
	c.token = &Token{Bearer: "valid-token", Expiration: time.Now().Add(time.Hour)}
	c.me = &Profile{ID: testMe}
	c.devices = []Device{{
		ID:          testDevice,
		LeftUserID:  testMe,
		RightUserID: testPartner,
		Timezone:    "America/Denver",
	}}
	return c, api
}

const tokenJSON = `{"access_token":"fresh-token","expires_in":3600,"userId":"user-me"}`

func TestTokenEndpointRateLimitIsNotRetriedInNestedLoops(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return fakeResponse{status: http.StatusTooManyRequests}
	})
	c.token = nil

	err := c.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected an error when the token endpoint keeps answering 429")
	}
	if got := len(api.calls(http.MethodPost, "/auth/tokens")); got != retryMaxAttempts {
		t.Fatalf("token endpoint called %d times, want %d", got, retryMaxAttempts)
	}
}

func TestBadCredentialsFailOnceWithoutLeakingTheResponseBody(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return fakeResponse{status: http.StatusUnauthorized, body: `{"echo":"hunter2"}`}
	})
	c.token = nil

	err := c.refreshToken(context.Background())
	if err == nil {
		t.Fatal("expected an error for rejected credentials")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("auth error leaks the response body: %v", err)
	}
	if got := len(api.calls(http.MethodPost, "/auth/tokens")); got != 1 {
		t.Fatalf("token endpoint called %d times, want 1", got)
	}
}

func TestAPIErrorsIncludeTheResponseBody(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		return fakeResponse{status: http.StatusBadRequest, body: `{"error":"level out of range"}`}
	})

	err := c.doJSON(context.Background(), http.MethodGet, c.appAPIURL+"/thing", nil, nil)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error %v is not an *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest ||
		!strings.Contains(err.Error(), "level out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExpiredTokenIsRefreshedBeforeTheRequest(t *testing.T) {
	c, api := newTestClient(t, func(r recordedRequest) fakeResponse {
		if r.Path == "/auth/tokens" {
			return ok(tokenJSON)
		}
		return ok(`{}`)
	})
	c.token.Expiration = time.Now().Add(time.Second)

	if err := c.doJSON(context.Background(), http.MethodGet, c.appAPIURL+"/thing", nil, nil); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	calls := api.calls(http.MethodGet, "/app/thing")
	if len(calls) != 1 || calls[0].Auth != "Bearer fresh-token" {
		t.Fatalf("request did not use the refreshed token: %+v", calls)
	}
}

func TestRejectedTokenTriggersOneReauthentication(t *testing.T) {
	c, api := newTestClient(t, func(r recordedRequest) fakeResponse {
		switch {
		case r.Path == "/auth/tokens":
			return ok(tokenJSON)
		case r.Auth == "Bearer fresh-token":
			return ok(`{"ok":true}`)
		default:
			return fakeResponse{status: http.StatusUnauthorized}
		}
	})

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.doJSON(context.Background(), http.MethodGet, c.appAPIURL+"/thing", nil, &out); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if !out.OK {
		t.Fatal("response was not decoded after re-authentication")
	}
	if got := len(api.calls(http.MethodPost, "/auth/tokens")); got != 1 {
		t.Fatalf("re-authenticated %d times, want 1", got)
	}
}

func TestServerErrorsAreRetriedUntilSuccess(t *testing.T) {
	attempts := 0
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		attempts++
		if attempts < 3 {
			return fakeResponse{status: http.StatusBadGateway}
		}
		return ok(`{}`)
	})

	if err := c.doJSON(context.Background(), http.MethodGet, c.appAPIURL+"/thing", nil, nil); err != nil {
		t.Fatalf("doJSON: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("made %d attempts, want 3", attempts)
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	attempts := 0
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		attempts++
		return fakeResponse{status: http.StatusNotFound}
	})

	if err := c.doJSON(context.Background(), http.MethodGet, c.appAPIURL+"/thing", nil, nil); err == nil {
		t.Fatal("expected an error for 404")
	}
	if attempts != 1 {
		t.Fatalf("made %d attempts, want 1", attempts)
	}
}

func TestRetryWaitsAtLeastAsLongAsTheServerAsked(t *testing.T) {
	const wait = 60 * time.Millisecond
	calls := 0
	start := time.Now()
	err := retryWithBackoff(context.Background(), time.Millisecond, func() error {
		calls++
		if calls == 1 {
			return retryAfterError{errors.New("slow down"), wait}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryWithBackoff: %v", err)
	}
	if elapsed := time.Since(start); elapsed < wait {
		t.Fatalf("retried after %s, want at least %s", elapsed, wait)
	}
}

func TestRetryStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retryWithBackoff(ctx, time.Hour, func() error {
		calls++
		cancel()
		return errors.New("transient")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times, want 1", calls)
	}
}

func TestMalformedAudioCatalogDoesNotPanic(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		return ok(`{"unexpected":true}`)
	})

	categories, err := c.GetAudioTracks(context.Background())
	if err != nil {
		t.Fatalf("GetAudioTracks: %v", err)
	}
	if len(categories) != 0 {
		t.Fatalf("got %d categories, want 0", len(categories))
	}
}

func TestPrimeRefusesWhileAlreadyPriming(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })
	c.devices[0].Priming = true

	if err := c.Prime(context.Background()); err == nil {
		t.Fatal("expected an error while the pod is already priming")
	}
	if len(api.requests) != 0 {
		t.Fatalf("sent %d requests, want none", len(api.requests))
	}
}

func TestPrimeNotifiesTheAuthenticatedUser(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })

	if err := c.Prime(context.Background()); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	calls := api.calls(http.MethodPost, "/app/v1/devices/"+testDevice+"/priming/tasks")
	if len(calls) != 1 {
		t.Fatalf("priming endpoint called %d times, want 1", len(calls))
	}
	notifications, _ := calls[0].Body["notifications"].(map[string]any)
	users, _ := notifications["users"].([]any)
	if len(users) != 1 || users[0] != testMe || notifications["meta"] != "rePriming" {
		t.Fatalf("unexpected priming body: %v", calls[0].Body)
	}
}

func TestCommandsFailClearlyWithoutADevice(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })
	c.devices = nil

	if err := c.Prime(context.Background()); err == nil {
		t.Fatal("Prime: expected an error with no devices")
	}
	if err := c.TurnOn(context.Background(), SideLeft); err == nil {
		t.Fatal("TurnOn: expected an error with no devices")
	}
	if err := c.Info(context.Background()); err == nil {
		t.Fatal("Info: expected an error with no devices")
	}
}

func TestInfoKeepsGoingWhenOnePayloadIsRefused(t *testing.T) {
	c, api := newTestClient(t, func(r recordedRequest) fakeResponse {
		if strings.HasSuffix(r.Path, "/alarms") {
			return fakeResponse{status: http.StatusForbidden, body: `{"message":"subscription required"}`}
		}
		return ok(`{}`)
	})

	err := c.Info(context.Background())
	if err == nil || !strings.Contains(err.Error(), "alarms") {
		t.Fatalf("error = %v, want the alarms failure reported", err)
	}
	if got := len(api.calls(http.MethodGet, "/app/v1/users/"+testMe+"/autopilotDetails")); got != 1 {
		t.Fatalf("payloads after the refused one were fetched %d times, want 1", got)
	}
}

func TestSubscriptionRefusalIsExplainedForEveryCommand(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		return fakeResponse{status: http.StatusForbidden, body: `{"message":"subscription required"}`}
	})

	_, listErr := c.ListAlarms(context.Background(), SideMine)
	awayErr := c.SetAway(context.Background(), SideMine, true)
	for name, err := range map[string]error{"alarm list": listErr, "away on": awayErr} {
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || !httpErr.SubscriptionRequired() {
			t.Fatalf("%s: error %v does not identify the subscription refusal", name, err)
		}
		if !strings.Contains(err.Error(), "refused by the Eight Sleep API") ||
			strings.Count(err.Error(), "subscription required") != 1 {
			t.Errorf("%s: unclear or repetitive message: %v", name, err)
		}
	}
}

func TestOtherForbiddenResponsesAreNotBlamedOnTheSubscription(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse {
		return fakeResponse{status: http.StatusForbidden, body: `{"message":"not your alarm"}`}
	})

	_, err := c.ListAlarms(context.Background(), SideMine)
	if err == nil || strings.Contains(err.Error(), "subscription") ||
		!strings.Contains(err.Error(), "not your alarm") {
		t.Fatalf("error = %v, want the server's own message and no subscription claim", err)
	}
}
