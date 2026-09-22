/*
Copyright © 2025 blacktop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/blacktop/clim8/pkg/eightsleep"
)

func TestStatusHighlightsWhatNeedsAttention(t *testing.T) {
	pod := eightsleep.PodStatus{
		DeviceID:     "device-1",
		Model:        "Pod 3",
		Online:       false,
		HasWater:     false,
		NeedsPriming: true,
		Unit:         eightsleep.Fahrenheit,
		Sides: []eightsleep.SideStatus{
			{Side: eightsleep.SideLeft, UserID: "me", Active: true, Level: -40, TargetLevel: -58,
				Temperature: 72, Target: 68, Activity: "smart:bedtime"},
			{Side: eightsleep.SideRight, UserID: "partner", Away: true, Active: true},
		},
	}

	var out bytes.Buffer
	if err := renderPodStatus(&out, pod, "me"); err != nil {
		t.Fatalf("renderPodStatus: %v", err)
	}
	if strings.Contains(out.String(), "Pod Pod") {
		t.Errorf("model name is duplicated:\n%s", out.String())
	}

	for _, want := range []string{
		"Pod 3 (device-1)\n", "OFFLINE", "LOW", "NEEDED", "never",
		"left*:", "ON  72°F (level -40) -> 68°F (level -58)  [smart:bedtime]",
		"right:", "AWAY",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status output is missing %q:\n%s", want, out.String())
		}
	}
}

func TestSideSummaryStates(t *testing.T) {
	tests := map[string]struct {
		side eightsleep.SideStatus
		unit eightsleep.UnitOfTemperature
		want string
	}{
		"unassigned": {eightsleep.SideStatus{}, eightsleep.Fahrenheit, "unassigned"},
		"off":        {eightsleep.SideStatus{UserID: "u"}, eightsleep.Fahrenheit, "OFF"},
		"away wins over active": {
			eightsleep.SideStatus{UserID: "u", Away: true, Active: true}, eightsleep.Fahrenheit, "AWAY"},
		"levels that round to the same temperature have no target arrow": {
			eightsleep.SideStatus{UserID: "u", Active: true, Level: -42, TargetLevel: -40,
				Temperature: 72, Target: 72},
			eightsleep.Fahrenheit, "ON  72°F (level -42)"},
		"settled temperature has no target arrow": {
			eightsleep.SideStatus{UserID: "u", Active: true, Level: 5, TargetLevel: 5,
				Temperature: 28, Target: 28},
			eightsleep.Celsius, "ON  28°C (level 5)"},
	}
	for name, tc := range tests {
		if got := sideSummary(tc.side, tc.unit); got != tc.want {
			t.Errorf("%s: sideSummary = %q, want %q", name, got, tc.want)
		}
	}
}

func TestAlarmRenderingOrdersWeekdaysAndHandlesNone(t *testing.T) {
	var weekday eightsleep.Alarm
	weekday.Repeat.Enabled = true
	weekday.Repeat.WeekDays = map[string]bool{"friday": true, "monday": true, "sunday": false}
	if got := repeatSummary(weekday); got != "mon,fri" {
		t.Errorf("repeatSummary = %q, want mon,fri", got)
	}
	if got := repeatSummary(eightsleep.Alarm{}); got != "once" {
		t.Errorf("repeatSummary(one-off) = %q, want once", got)
	}

	var out bytes.Buffer
	if err := renderAlarms(&out, nil); err != nil || !strings.Contains(out.String(), "No alarms") {
		t.Errorf("renderAlarms(nil) = %q, %v", out.String(), err)
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatSeconds(27000); got != "7h30m" {
		t.Errorf("formatSeconds(27000) = %q, want 7h30m", got)
	}
	if got := formatSeconds(0); got != "0h00m" {
		t.Errorf("formatSeconds(0) = %q, want 0h00m", got)
	}
	if got := formatTime(time.Time{}); got != "never" {
		t.Errorf("formatTime(zero) = %q, want never", got)
	}
}

func TestScheduleItemsValidateTheirSide(t *testing.T) {
	valid := []ScheduleItem{
		{Time: "22:00", Action: "on"},
		{Time: "22:00", Action: "on", Side: "both"},
		{Time: "22:15", Action: "temp", Temperature: "68F", Side: "right"},
	}
	for _, item := range valid {
		if err := validateScheduleItem(item); err != nil {
			t.Errorf("validateScheduleItem(%+v): %v", item, err)
		}
	}
	invalid := []ScheduleItem{
		{Time: "22:00", Action: "on", Side: "middle"},
		{Time: "22:00", Action: "temp", Side: "left"},
		{Time: "25:00", Action: "off"},
		{Time: "22:00", Action: "nap"},
	}
	for _, item := range invalid {
		if err := validateScheduleItem(item); err == nil {
			t.Errorf("validateScheduleItem(%+v): expected an error", item)
		}
	}
}

func temperatureState(t *testing.T, stateType string, level int) eightsleep.TemperatureState {
	t.Helper()
	payload := fmt.Sprintf(`{"devices":[{"currentLevel":%d,"currentState":{"type":%q}}]}`,
		level, stateType)
	var state eightsleep.TemperatureState
	if err := json.Unmarshal([]byte(payload), &state); err != nil {
		t.Fatalf("decode temperature state: %v", err)
	}
	return state
}

func TestDeviceStateMatchesRequiresEverySide(t *testing.T) {
	level := eightsleep.TempToHeatingLevel(68, eightsleep.Fahrenheit)
	item := &ScheduleItem{Action: "temp", Temperature: "68F", Side: "both"}

	both := []eightsleep.TemperatureState{
		temperatureState(t, "smart", level), temperatureState(t, "smart", level+1),
	}
	if ok, err := deviceStateMatches(both, item); err != nil || !ok {
		t.Errorf("matching sides: got %t, %v; want true", ok, err)
	}

	oneOff := []eightsleep.TemperatureState{
		temperatureState(t, "smart", level), temperatureState(t, "off", level),
	}
	if ok, err := deviceStateMatches(oneOff, item); err != nil || ok {
		t.Errorf("one side off: got %t, %v; want false", ok, err)
	}

	drifted := []eightsleep.TemperatureState{temperatureState(t, "smart", level+10)}
	if ok, err := deviceStateMatches(drifted, item); err != nil || ok {
		t.Errorf("drifted level: got %t, %v; want false", ok, err)
	}

	if _, err := deviceStateMatches(nil, item); err == nil {
		t.Error("expected an error when there are no states to compare")
	}
}
