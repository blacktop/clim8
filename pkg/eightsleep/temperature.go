package eightsleep

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// temperatureTolerance is the acceptable difference in heating levels for validation
const temperatureTolerance = 2

// SleepStage names one of the Autopilot temperature phases.
type SleepStage string

const (
	StageBedtime SleepStage = "bedTimeLevel"
	StageInitial SleepStage = "initialSleepLevel"
	StageFinal   SleepStage = "finalSleepLevel"
)

// ParseSleepStage parses a --stage value.
func ParseSleepStage(s string) (SleepStage, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "bedtime":
		return StageBedtime, nil
	case "initial":
		return StageInitial, nil
	case "final":
		return StageFinal, nil
	default:
		return "", fmt.Errorf("invalid stage %q (must be bedtime, initial or final)", s)
	}
}

// ParseTemperature parses a temperature string like "68F" or "24C" into value and unit
func ParseTemperature(degrees string) (int, UnitOfTemperature, error) {
	var unit UnitOfTemperature
	switch {
	case strings.HasSuffix(degrees, "C"):
		unit = Celsius
	case strings.HasSuffix(degrees, "F"):
		unit = Fahrenheit
	default:
		return 0, "", fmt.Errorf("invalid temperature format: %s (must end with C or F)", degrees)
	}
	temp, err := strconv.Atoi(strings.TrimRight(degrees, "CF"))
	if err != nil {
		return 0, "", fmt.Errorf("invalid temperature value: %s", degrees)
	}
	return temp, unit, nil
}

// podURL is the per-device temperature route; it answers with a `devices` list.
func (c *Client) podURL(userID string) string {
	return c.appAPIURL + "/v1/users/" + userID + "/temperature/pod?ignoreDeviceErrors=false"
}

// temperatureURL is the per-user temperature route, which owns timed and Autopilot levels.
func (c *Client) temperatureURL(userID string) string {
	return c.appAPIURL + "/v1/users/" + userID + "/temperature"
}

func (c *Client) putPod(ctx context.Context, userID string, body any) (*TemperatureState, error) {
	var resp TemperatureState
	if err := c.doJSON(ctx, http.MethodPut, c.podURL(userID), body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Devices) == 0 {
		return nil, fmt.Errorf("no devices in temperature response for user %s", userID)
	}
	return &resp, nil
}

func (c *Client) TurnOn(ctx context.Context, side Side) error {
	return c.setPower(ctx, side, true)
}

func (c *Client) TurnOff(ctx context.Context, side Side) error {
	return c.setPower(ctx, side, false)
}

func (c *Client) setPower(ctx context.Context, side Side, on bool) error {
	userIDs, err := c.userIDs(side)
	if err != nil {
		return err
	}
	state, verb := "off", "turn off"
	if on {
		state, verb = "smart", "turn on"
	}
	body := map[string]any{"currentState": map[string]string{"type": state}}
	for _, userID := range userIDs {
		resp, err := c.putPod(ctx, userID, body)
		if err != nil {
			return fmt.Errorf("failed to %s: %w", verb, err)
		}
		for _, device := range resp.Devices {
			if isOff := device.CurrentState.Type == "off"; isOff == on {
				return fmt.Errorf("failed to %s device %s: state is %q",
					verb, device.Device.DeviceID, device.CurrentState.Type)
			}
		}
	}
	return nil
}

// GetTemperatureStates returns the temperature state of every user selected by side.
func (c *Client) GetTemperatureStates(ctx context.Context, side Side) ([]TemperatureState, error) {
	userIDs, err := c.userIDs(side)
	if err != nil {
		return nil, err
	}
	states := make([]TemperatureState, 0, len(userIDs))
	for _, userID := range userIDs {
		var resp TemperatureState
		if err := c.doJSON(ctx, http.MethodGet, c.podURL(userID), nil, &resp); err != nil {
			return nil, fmt.Errorf("failed to get temperature state: %w", err)
		}
		states = append(states, resp)
	}
	return states, nil
}

// SetTemperature sets the bed temperature for the selected side(s). A positive hold keeps the
// temperature for that long, after which the pod stops holding it (a side with nothing else
// scheduled turns off); zero holds it indefinitely.
// The side must already be on: the API accepts a level for a side that is off and ignores it.
func (c *Client) SetTemperature(
	ctx context.Context, side Side, degrees string, hold time.Duration,
) error {
	temp, unit, err := ParseTemperature(degrees)
	if err != nil {
		return err
	}
	if hold < 0 {
		return fmt.Errorf("invalid hold duration %s (must not be negative)", hold)
	}
	userIDs, err := c.userIDs(side)
	if err != nil {
		return err
	}
	level := TempToHeatingLevel(temp, unit)
	for _, userID := range userIDs {
		if err := c.setLevel(ctx, userID, level); err != nil {
			return err
		}
		if hold == 0 {
			continue
		}
		body := map[string]any{
			"timeBased": map[string]int{"level": level, "durationSeconds": int(hold.Seconds())},
		}
		if err := c.doJSON(ctx, http.MethodPut, c.temperatureURL(userID), body, nil); err != nil {
			return fmt.Errorf("failed to set temperature hold: %w", err)
		}
	}
	return nil
}

func (c *Client) setLevel(ctx context.Context, userID string, level int) error {
	resp, err := c.putPod(ctx, userID, map[string]any{"currentLevel": level})
	if err != nil {
		return fmt.Errorf("failed to set temperature: %w", err)
	}
	for _, device := range resp.Devices {
		if diff := abs(device.CurrentLevel - level); diff > temperatureTolerance {
			return fmt.Errorf(
				"failed to set temperature on device %s: expected level %d, got %d "+
					"(diff %d > tolerance %d)",
				device.Device.DeviceID, level, device.CurrentLevel, diff, temperatureTolerance)
		}
	}

	// Re-verify by querying actual state
	var verify TemperatureState
	if err := c.doJSON(ctx, http.MethodGet, c.podURL(userID), nil, &verify); err != nil {
		log.Warn("failed to verify temperature state after set", "err", err)
		return nil
	}
	for _, device := range verify.Devices {
		if diff := abs(device.CurrentLevel - level); diff > temperatureTolerance {
			log.Warn("temperature verification mismatch",
				"device", device.Device.DeviceID,
				"expected", level,
				"actual", device.CurrentLevel,
				"diff", diff)
		}
	}
	return nil
}

// SetStageTemperature sets the Autopilot temperature for one sleep stage.
func (c *Client) SetStageTemperature(
	ctx context.Context, side Side, stage SleepStage, degrees string,
) error {
	temp, unit, err := ParseTemperature(degrees)
	if err != nil {
		return err
	}
	userIDs, err := c.userIDs(side)
	if err != nil {
		return err
	}
	level := TempToHeatingLevel(temp, unit)
	for _, userID := range userIDs {
		var current struct {
			Smart map[string]any `json:"smart"`
		}
		url := c.temperatureURL(userID)
		if err := c.doJSON(ctx, http.MethodGet, url, nil, &current); err != nil {
			return fmt.Errorf("failed to get Autopilot levels: %w", err)
		}
		if current.Smart == nil {
			return fmt.Errorf("temperature response for user %s has no Autopilot levels", userID)
		}
		current.Smart[string(stage)] = level
		body := map[string]any{"smart": current.Smart}
		if err := c.doJSON(ctx, http.MethodPut, url, body, nil); err != nil {
			return fmt.Errorf("failed to set %s: %w", stage, err)
		}
	}
	return nil
}
