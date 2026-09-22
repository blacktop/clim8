package eightsleep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"
)

// alarmComputedFields are set by the server and must not be sent back on update.
var alarmComputedFields = []string{
	"nextTimestamp", "startTimestamp", "endTimestamp", "dismissedUntil", "snoozedUntil",
}

// Alarm is one alarm from the alarms API. The API only accepts whole objects on update, so the
// complete payload is kept alongside the fields clim8 reads.
type Alarm struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
	Repeat  struct {
		Enabled  bool            `json:"enabled"`
		WeekDays map[string]bool `json:"weekDays"`
	} `json:"repeat"`
	Snoozing       bool      `json:"snoozing"`
	NextTimestamp  time.Time `json:"nextTimestamp"`
	StartTimestamp time.Time `json:"startTimestamp"`
	EndTimestamp   time.Time `json:"endTimestamp"`

	raw map[string]any
}

func (a *Alarm) UnmarshalJSON(data []byte) error {
	type fields Alarm
	if err := json.Unmarshal(data, (*fields)(a)); err != nil {
		return err
	}
	return json.Unmarshal(data, &a.raw)
}

func (a Alarm) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.raw)
}

// IsRinging reports whether the alarm is going off or snoozed at the given time.
func (a Alarm) IsRinging(now time.Time) bool {
	if a.Snoozing {
		return true
	}
	if a.StartTimestamp.IsZero() || a.EndTimestamp.IsZero() {
		return false
	}
	return !now.Before(a.StartTimestamp) && !now.After(a.EndTimestamp)
}

// AlarmOptions describes a new one-off alarm.
type AlarmOptions struct {
	// Time is the local wake time as HH:MM.
	Time string
	// VibrationLevel is 0-100; zero disables vibration.
	VibrationLevel int
	// ThermalLevel is the -100..100 heating level to wake with.
	ThermalLevel int
}

func (c *Client) alarmsURL(version, userID string) string {
	return c.appAPIURL + "/" + version + "/users/" + userID + "/alarms"
}

// ListAlarms returns the alarms of the user on the selected side.
func (c *Client) ListAlarms(ctx context.Context, side Side) ([]Alarm, error) {
	userID, err := c.userID(side)
	if err != nil {
		return nil, err
	}
	return c.listAlarms(ctx, userID)
}

func (c *Client) listAlarms(ctx context.Context, userID string) ([]Alarm, error) {
	var resp struct {
		Alarms []Alarm `json:"alarms"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.alarmsURL("v2", userID), nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to list alarms: %w", err)
	}
	return resp.Alarms, nil
}

// wakeTime returns Time in the HH:MM:SS form the API expects.
func (o AlarmOptions) wakeTime() (string, error) {
	wake, err := time.Parse("15:04", o.Time)
	if err != nil {
		return "", fmt.Errorf("invalid alarm time %q (must be HH:MM, 24-hour)", o.Time)
	}
	return wake.Format("15:04:05"), nil
}

// Validate reports the first problem with the options. Callers can use it to reject bad input
// before authenticating.
func (o AlarmOptions) Validate() error {
	if _, err := o.wakeTime(); err != nil {
		return err
	}
	if o.VibrationLevel < 0 || o.VibrationLevel > 100 {
		return fmt.Errorf("invalid vibration level %d (must be 0-100)", o.VibrationLevel)
	}
	if o.ThermalLevel < -100 || o.ThermalLevel > 100 {
		return fmt.Errorf("invalid thermal level %d (must be -100 to 100)", o.ThermalLevel)
	}
	return nil
}

// CreateAlarm creates a one-off alarm for the user on the selected side.
func (c *Client) CreateAlarm(ctx context.Context, side Side, opts AlarmOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	wake, err := opts.wakeTime()
	if err != nil {
		return err
	}
	userID, err := c.userID(side)
	if err != nil {
		return err
	}
	body := map[string]any{
		"time":    wake,
		"enabled": true,
		"vibration": map[string]any{
			"enabled":    opts.VibrationLevel > 0,
			"powerLevel": opts.VibrationLevel,
			"pattern":    "RISE",
		},
		"thermal": map[string]any{
			"enabled": true,
			"level":   opts.ThermalLevel,
		},
	}
	if err := c.doJSON(ctx, http.MethodPost, c.alarmsURL("v1", userID), body, nil); err != nil {
		return fmt.Errorf("failed to create alarm: %w", err)
	}
	return nil
}

// SetAlarmEnabled enables or disables an existing alarm.
func (c *Client) SetAlarmEnabled(ctx context.Context, side Side, alarmID string, on bool) error {
	userID, err := c.userID(side)
	if err != nil {
		return err
	}
	alarms, err := c.listAlarms(ctx, userID)
	if err != nil {
		return err
	}
	for _, alarm := range alarms {
		if alarm.ID != alarmID {
			continue
		}
		body := maps.Clone(alarm.raw)
		for _, key := range alarmComputedFields {
			delete(body, key)
		}
		body["enabled"] = on
		url := c.alarmsURL("v1", userID) + "/" + alarmID
		if err := c.doJSON(ctx, http.MethodPut, url, body, nil); err != nil {
			return fmt.Errorf("failed to update alarm: %w", err)
		}
		return nil
	}
	return fmt.Errorf("alarm %s not found (run `clim8 alarm list` for valid ids)", alarmID)
}

// DismissAlarm dismisses an alarm. An empty alarmID selects the alarm that is ringing.
func (c *Client) DismissAlarm(ctx context.Context, side Side, alarmID string) error {
	body := map[string]any{"ignoreDeviceErrors": false}
	return c.alarmAction(ctx, side, alarmID, "dismiss", body)
}

// SnoozeAlarm snoozes an alarm. An empty alarmID selects the alarm that is ringing.
func (c *Client) SnoozeAlarm(ctx context.Context, side Side, alarmID string, minutes int) error {
	if minutes <= 0 {
		return fmt.Errorf("invalid snooze duration %d (must be at least 1 minute)", minutes)
	}
	body := map[string]any{"snoozeMinutes": minutes, "ignoreDeviceErrors": false}
	return c.alarmAction(ctx, side, alarmID, "snooze", body)
}

func (c *Client) alarmAction(
	ctx context.Context, side Side, alarmID, action string, body map[string]any,
) error {
	userID, err := c.userID(side)
	if err != nil {
		return err
	}
	if alarmID == "" {
		alarms, err := c.listAlarms(ctx, userID)
		if err != nil {
			return err
		}
		alarmID, err = ringingAlarmID(alarms, time.Now())
		if err != nil {
			return err
		}
	}
	url := c.alarmsURL("v1", userID) + "/" + alarmID + "/" + action
	if err := c.doJSON(ctx, http.MethodPut, url, body, nil); err != nil {
		return fmt.Errorf("failed to %s alarm: %w", action, err)
	}
	return nil
}

func ringingAlarmID(alarms []Alarm, now time.Time) (string, error) {
	for _, alarm := range alarms {
		if alarm.IsRinging(now) {
			return alarm.ID, nil
		}
	}
	return "", errors.New("no alarm is ringing; pass an alarm id (see `clim8 alarm list`)")
}
