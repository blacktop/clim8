package eightsleep

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SleepMetric is a biometric reading for one night.
type SleepMetric struct {
	Current float64 `json:"current"`
	Average float64 `json:"average"`
}

// SleepDay is one night from the trends API. Durations are in seconds.
type SleepDay struct {
	Day              string    `json:"day"`
	Score            float64   `json:"score"`
	Processing       bool      `json:"processing"`
	TossAndTurns     float64   `json:"tnt"`
	PresenceDuration float64   `json:"presenceDuration"`
	SleepDuration    float64   `json:"sleepDuration"`
	LightDuration    float64   `json:"lightDuration"`
	DeepDuration     float64   `json:"deepDuration"`
	RemDuration      float64   `json:"remDuration"`
	PresenceStart    time.Time `json:"presenceStart"`
	PresenceEnd      time.Time `json:"presenceEnd"`
	Quality          struct {
		Total           float64     `json:"total"`
		HRV             SleepMetric `json:"hrv"`
		HeartRate       SleepMetric `json:"heartRate"`
		RespiratoryRate SleepMetric `json:"respiratoryRate"`
		TempBedC        SleepMetric `json:"tempBedC"`
		TempRoomC       SleepMetric `json:"tempRoomC"`
	} `json:"sleepQualityScore"`
	Routine struct {
		Total float64 `json:"total"`
	} `json:"sleepRoutineScore"`
}

func (c *Client) trendsURL(userID string, from, to time.Time) (string, error) {
	device, err := c.primaryDevice()
	if err != nil {
		return "", err
	}
	tz := device.Timezone
	if tz == "" {
		tz = c.tz.String()
	}
	q := url.Values{}
	q.Set("tz", tz)
	q.Set("from", from.Format(time.DateOnly))
	q.Set("to", to.Format(time.DateOnly))
	q.Set("include-main", "false")
	q.Set("include-all-sessions", "true")
	q.Set("model-version", "v2")
	return c.clientAPIURL + "/users/" + userID + "/trends?" + q.Encode(), nil
}

// SleepDays returns the nights between from and to (inclusive) for the user on the selected side.
func (c *Client) SleepDays(ctx context.Context, side Side, from, to time.Time) ([]SleepDay, error) {
	if to.Before(from) {
		return nil, fmt.Errorf("invalid date range: %s is after %s",
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}
	userID, err := c.userID(side)
	if err != nil {
		return nil, err
	}
	reqURL, err := c.trendsURL(userID, from, to)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Days []SleepDay `json:"days"`
	}
	if err := c.doJSON(ctx, http.MethodGet, reqURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch sleep data: %w", err)
	}
	return resp.Days, nil
}
