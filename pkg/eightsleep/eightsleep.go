package eightsleep

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/log"
)

const (
	clientAPIURL = "https://client-api.8slp.net/v1"
	appAPIURL    = "https://app-api.8slp.net"
	authURL      = "https://auth-api.8slp.net/v1/tokens"

	knownClientID     = "0894c7f33bb94800a03f1f4df13a4f38"
	knownClientSecret = "f0954a3ed5763ba3d06834c73731a32f15f168f47d4f164751275def86db0c76"

	tokenRefreshBufferSec = 120
	defaultTimeoutSec     = 240

	MIN_TEMP_F = 55
	MAX_TEMP_F = 110
	MIN_TEMP_C = 13
	MAX_TEMP_C = 44
)

var POSSIBLE_SLEEP_STAGES = []string{"bedTimeLevel", "initialSleepLevel", "finalSleepLevel"}

type Client struct {
	mu sync.RWMutex

	email, password string
	tz              *time.Location

	clientID, clientSecret string

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
	return &Client{
		email:        email,
		password:     password,
		tz:           loc,
		clientID:     knownClientID,
		clientSecret: knownClientSecret,
		http: &http.Client{
			Timeout: time.Second * defaultTimeoutSec,
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
	if err := c.fetchDevices(ctx); err != nil {
		return fmt.Errorf("failed to fetch devices: %w", err)
	}
	return nil
}

func (c *Client) Stop() { /* nothing to close right now */ }

func (c *Client) RoomTemperature(ctx context.Context) (float64, error) {
	panic("not implemented")
	// TODO: get trends and calculate room temperature average (average both sides if both are active)
}

func (c *Client) TurnOn(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/users/%s/temperature/pod?ignoreDeviceErrors=false", appAPIURL, c.me.ID)
	body := map[string]any{
		"currentState": map[string]string{"type": "smart"},
	}
	var resp TemperatureState
	if err := c.doJSON(ctx, http.MethodPut, url, body, &resp); err != nil {
		return fmt.Errorf("failed to turn on device: %w", err)
	}

	for _, device := range resp.Devices {
		if device.CurrentState.Type == "off" {
			return fmt.Errorf("failed to turn on device %s: %s", device.Device.DeviceID, device.CurrentState.Type)
		}
	}

	return nil
}

func (c *Client) TurnOff(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/users/%s/temperature/pod?ignoreDeviceErrors=false", appAPIURL, c.me.ID)
	body := map[string]any{
		"currentState": map[string]string{"type": "off"},
	}
	var resp TemperatureState
	if err := c.doJSON(ctx, http.MethodPut, url, body, &resp); err != nil {
		return fmt.Errorf("failed to turn off device: %w", err)
	}

	for _, device := range resp.Devices {
		if device.CurrentState.Type != "off" {
			return fmt.Errorf("failed to turn off device %s: %s", device.Device.DeviceID, device.CurrentState.Type)
		}
	}

	return nil
}

func (c *Client) GetTemperatureState(ctx context.Context) (*TemperatureState, error) {
	url := fmt.Sprintf("%s/v1/users/%s/temperature/pod?ignoreDeviceErrors=false", appAPIURL, c.me.ID)
	var resp TemperatureState
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to get temperature state: %w", err)
	}
	return &resp, nil
}

func (c *Client) SetTemperature(ctx context.Context, degrees string) error {
	// parse degrees
	var unit UnitOfTemperature
	switch {
	case strings.HasSuffix(degrees, "C"):
		unit = Celsius
	case strings.HasSuffix(degrees, "F"):
		unit = Fahrenheit
	default:
		return fmt.Errorf("invalid temperature format: %s (must end with C or F)", degrees)
	}
	temp, err := strconv.Atoi(strings.Trim(degrees, "CF"))
	if err != nil {
		return fmt.Errorf("invalid temperature value: %s", degrees)
	}

	url := fmt.Sprintf("%s/v1/users/%s/temperature/pod?ignoreDeviceErrors=false", appAPIURL, c.me.ID)
	body := map[string]any{
		"currentLevel": TempToHeatingLevel(temp, unit),
	}
	var resp TemperatureState
	if err := c.doJSON(ctx, http.MethodPut, url, body, &resp); err != nil {
		return fmt.Errorf("failed to set temperature: %w", err)
	}

	for _, device := range resp.Devices {
		if device.CurrentLevel != TempToHeatingLevel(temp, unit) {
			return fmt.Errorf("failed to set temperature on device %s: %s", device.Device.DeviceID, device.CurrentState.Type)
		}
	}

	return nil
}

func (c *Client) Info(ctx context.Context) (map[string]any, error) {
	if err := c.fetchTrends(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch trends: %w", err)
	}
	if err := c.fetchIntervals(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch intervals: %w", err)
	}
	if err := c.fetchRoutines(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch routines: %w", err)
	}
	if err := c.fetchHealthSurveyTestDrive(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch health survey test drive: %w", err)
	}
	if err := c.fetchSubscriptions(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch subscriptions: %w", err)
	}
	if err := c.fetchAutopilotDetails(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch autopilot details: %w", err)
	}
	return nil, nil
}

func (c *Client) GetReleaseFeatures(ctx context.Context) (map[string]any, error) {
	url := appAPIURL + "/v1/users/" + c.me.ID + "/release-features"
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
		return nil, fmt.Errorf("failed to fetch release features: %w", err)
	}
	return data, nil
}

func (c *Client) GetAudioTracks(ctx context.Context) (map[string]any, error) {
	url := appAPIURL + "/v1/audio/categories"
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
		return nil, fmt.Errorf("failed to fetch audio categories: %w", err)
	}
	categories := data["categories"].([]any)
	for idx, category := range categories {
		url := appAPIURL + "/v1/users/" + c.me.ID + "/audio/tracks?category=" + category.(map[string]any)["id"].(string)
		var tracks map[string]any
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &tracks); err != nil {
			return nil, fmt.Errorf("failed to fetch audio tracks: %w", err)
		}
		categories[idx].(map[string]any)["tracks"] = tracks["tracks"]
		// for _, track := range tracks["tracks"].([]any) {
		// 	url := appAPIURL + "/v1/audio/track/" + track.(map[string]any)["id"].(string)
		// 	var trackDetails map[string]any
		// 	if err := c.doJSON(ctx, http.MethodGet, url, nil, &trackDetails); err != nil {
		// 		return nil, fmt.Errorf("failed to fetch audio track details: %w", err)
		// 	}
		// }
	}
	data["categories"] = categories
	return data, nil
}

func (c *Client) SetAlarm(ctx context.Context, time string) error {
	url := fmt.Sprintf("%s/v2/users/%s/routines/%s", appAPIURL, c.me.ID, "1234")
	body := map[string]any{
		"id":      "1234",
		"alarms":  []any{},
		"days":    []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
		"enabled": true,
		"bedtime": map[string]any{
			"time":      "22:30:00",
			"dayOffset": "MinusOne",
		},
		"alarmsToCreate": []map[string]any{
			{
				"enabled":              true,
				"disabledIndividually": false,
				"timeWithOffset": map[string]any{
					"time":      time,
					"dayOffset": "Zero",
				},
				"settings": map[string]any{
					"vibration": map[string]any{
						"enabled":    true,
						"powerLevel": 50,
						"pattern":    "rise",
					},
					"thermal": map[string]any{
						"enabled": true,
						"level":   20,
					},
				},
				"dismissedUntil": "1970-01-01T00:00:00Z",
				"snoozedUntil":   "1970-01-01T00:00:00Z",
			},
		},
	}
	var resp map[string]any
	if err := c.doJSON(ctx, http.MethodPut, url, body, &resp); err != nil {
		return fmt.Errorf("failed to set alarm: %w", err)
	}
	// TODO: check if alarm was set successfully via response JSON
	if log.GetLevel() == log.DebugLevel {
		if err := prettyPrint(resp); err != nil {
			return fmt.Errorf("failed to pretty print response: %w", err)
		}
	}
	return nil
}

func (c *Client) Status(ctx context.Context) {
	for _, device := range c.devices {
		if device.LeftKelvin.Active || device.RightKelvin.Active {
			fmt.Printf("Eight Sleep is ON\n")
			if device.LeftKelvin.Active {
				fmt.Printf("Left side: %s\n", device.LeftKelvin.CurrentActivity)
				if device.LeftHeatingLevel < device.LeftTargetHeatingLevel {
					fmt.Printf("Left side target heating to level %d from %d\n", device.LeftTargetHeatingLevel, device.LeftHeatingLevel)
				} else if device.LeftHeatingLevel > device.LeftTargetHeatingLevel {
					fmt.Printf("Left side target cooling to level %d from %d\n", device.LeftTargetHeatingLevel, device.LeftHeatingLevel)
				}
			}
			if device.RightKelvin.Active {
				fmt.Printf("Right side: %s\n", device.RightKelvin.CurrentActivity)
				if device.RightHeatingLevel < device.RightTargetHeatingLevel {
					fmt.Printf("Right side target heating to level %d from %d\n", device.RightTargetHeatingLevel, device.RightHeatingLevel)
				} else if device.RightHeatingLevel > device.RightTargetHeatingLevel {
					fmt.Printf("Right side target cooling to level %d from %d\n", device.RightTargetHeatingLevel, device.RightHeatingLevel)
				}
			}
		} else {
			fmt.Printf("Eight Sleep is OFF\n")
		}
	}
}

/* -------------------- internal helpers -------------------- */

func (c *Client) headers() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	h.Set("Accept-Encoding", "gzip")
	h.Set("User-Agent", "okhttp/4.9.3")
	c.mu.RLock()
	if c.token != nil {
		h.Set("Authorization", "Bearer "+c.token.Bearer)
	}
	c.mu.RUnlock()
	return h
}

func (c *Client) refreshToken(ctx context.Context) error {
	c.mu.RLock()
	needsRefresh := c.token == nil || time.Until(c.token.Expiration) < time.Second*tokenRefreshBufferSec
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
	if err := c.doJSON(ctx, http.MethodPost, authURL, body, &res); err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	c.mu.Lock()
	c.token = &Token{
		Bearer:     res.AccessToken,
		Expiration: time.Now().Add(time.Duration(res.ExpiresIn) * time.Second),
		MainID:     res.UserID,
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchProfile(ctx context.Context) error {
	url := clientAPIURL + "/users/me"
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

func (c *Client) fetchDevices(ctx context.Context) error {
	for _, device := range c.me.Devices {
		url := clientAPIURL + "/devices/" + device
		var data struct {
			Result Device `json:"result"`
		}
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &data); err != nil {
			return fmt.Errorf("failed to fetch device %s: %w", device, err)
		}
		c.mu.Lock()
		c.devices = append(c.devices, data.Result)
		c.mu.Unlock()
	}
	return nil
}

func (c *Client) fetchTrends(ctx context.Context) error {
	url, err := url.Parse(clientAPIURL + "/users/" + c.me.ID + "/trends")
	if err != nil {
		return fmt.Errorf("failed to parse trends URL: %w", err)
	}
	q := url.Query()
	q.Add("tz", c.devices[0].Timezone)
	q.Add("from", time.Now().AddDate(0, 0, -1).Format(time.DateOnly))
	q.Add("to", time.Now().Format(time.DateOnly))
	q.Add("include-main", "false")
	q.Add("include-all-sessions", "true")
	q.Add("model-version", "v2")
	url.RawQuery = q.Encode()
	// var data struct {
	// 	Days          []any  `json:"days"`
	// 	ModelVersion  string `json:"modelVersion"`
	// 	SfsCalculator string `json:"sfsCalculator"`
	// }
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch trends: %w", err)
	}
	c.mu.Lock()
	log.Info("TRENDS")
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchIntervals(ctx context.Context) error {
	url, err := url.Parse(clientAPIURL + "/users/" + c.me.ID + "/intervals")
	if err != nil {
		return fmt.Errorf("failed to parse intervals URL: %w", err)
	}
	// var data struct {
	// 	Settings struct {
	// 		Routines     []any `json:"routines"`
	// 		OneOffAlarms []any `json:"oneOffAlarms"`
	// 	} `json:"settings"`
	// 	State struct {
	// 		Status    string `json:"status"`
	// 		NextAlarm struct {
	// 			NextTimestamp string `json:"nextTimestamp"`
	// 		} `json:"nextAlarm"`
	// 	} `json:"state"`
	// }
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch intervals: %w", err)
	}
	c.mu.Lock()
	log.Info("INTERVALS")
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchRoutines(ctx context.Context) error {
	url, err := url.Parse(appAPIURL + "/v2/users/" + c.me.ID + "/routines")
	if err != nil {
		return fmt.Errorf("failed to parse routines URL: %w", err)
	}
	// var data struct {
	// 	Settings struct {
	// 		Routines     []any `json:"routines"`
	// 		OneOffAlarms []any `json:"oneOffAlarms"`
	// 	} `json:"settings"`
	// 	State struct {
	// 		Status    string `json:"status"`
	// 		NextAlarm struct {
	// 			NextTimestamp string `json:"nextTimestamp"`
	// 		} `json:"nextAlarm"`
	// 	} `json:"state"`
	// }
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch routines: %w", err)
	}
	c.mu.Lock()
	log.Info("ROUTINES")
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchHealthSurveyTestDrive(ctx context.Context) error {
	url, err := url.Parse(appAPIURL + "/v1/health-survey/test-drive")
	if err != nil {
		return fmt.Errorf("failed to parse health survey test drive URL: %w", err)
	}
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch routines: %w", err)
	}
	c.mu.Lock()
	log.Info("HEALTH SURVEY TEST DRIVE")
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchSubscriptions(ctx context.Context) error {
	url, err := url.Parse(appAPIURL + "/v3/users/" + c.me.ID + "/subscriptions")
	if err != nil {
		return fmt.Errorf("failed to parse subscriptions URL: %w", err)
	}
	var data map[string]any
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch subscriptions: %w", err)
	}
	c.mu.Lock()
	log.Info("SUBSCRIPTIONS")
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchAutopilotDetails(ctx context.Context) error {
	url, err := url.Parse(appAPIURL + "/v1/users/" + c.me.ID + "/autopilotDetails")
	if err != nil {
		return fmt.Errorf("failed to parse autopilot details URL: %w", err)
	}
	var data map[string]any
	log.Info("AUTOPILOT DETAILS")
	if err := c.doJSON(ctx, http.MethodGet, url.String(), nil, &data); err != nil {
		return fmt.Errorf("failed to fetch autopilot details: %w", err)
	}
	c.mu.Lock()
	if err := prettyPrint(data); err != nil {
		return fmt.Errorf("failed to pretty print response: %w", err)
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, url string, payload any, out any) error {
	var body *bytes.Reader

	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header = c.headers()

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute %s request: %w", method, err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, res.Status)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if res.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		data, err = io.ReadAll(gzipReader)
		if err != nil {
			return fmt.Errorf("failed to read gzipped response body: %w", err)
		}
	}

	log.Debugf("HTTP %s %s: %d\n%s", method, url, res.StatusCode, string(data))

	return json.NewDecoder(bytes.NewReader(data)).Decode(out)
}

func prettyPrint(data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %v", err)
	}
	if err := quick.Highlight(os.Stdout, string(jsonData)+"\n", "json", "terminal256", "nord"); err != nil {
		return fmt.Errorf("failed to highlight json: %v", err)
	}
	return nil
}
