package eightsleep

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

const alarmsListPath = "/app/v2/users/" + testMe + "/alarms"
const alarmsWritePath = "/app/v1/users/" + testMe + "/alarms"

const alarmsJSON = `{"alarms":[
	{"id":"alarm-weekday","enabled":true,"time":"07:00:00",
	 "repeat":{"enabled":true,"weekDays":{"monday":true,"friday":true}},
	 "thermal":{"enabled":true,"temperature":-10},
	 "futureField":{"kept":true},
	 "snoozing":false,"snoozedUntil":null,"dismissedUntil":null,
	 "nextTimestamp":"2026-09-22T13:00:00Z",
	 "startTimestamp":"2026-09-22T12:55:00Z","endTimestamp":"2026-09-22T13:30:00Z"},
	{"id":"alarm-once","enabled":false,"time":"09:15:00","repeat":{"enabled":false},
	 "snoozing":false,"nextTimestamp":null,"startTimestamp":null,"endTimestamp":null}
]}`

func alarmServer(r recordedRequest) fakeResponse {
	if r.Method == http.MethodGet {
		return ok(alarmsJSON)
	}
	return ok(`{}`)
}

func TestListAlarmsDecodesNullTimestamps(t *testing.T) {
	c, _ := newTestClient(t, alarmServer)

	alarms, err := c.ListAlarms(context.Background(), SideMine)
	if err != nil {
		t.Fatalf("ListAlarms: %v", err)
	}
	if len(alarms) != 2 {
		t.Fatalf("got %d alarms, want 2", len(alarms))
	}
	if !alarms[0].Repeat.WeekDays["friday"] || alarms[0].NextTimestamp.IsZero() {
		t.Errorf("weekday alarm decoded incorrectly: %+v", alarms[0])
	}
	if alarms[1].Enabled || !alarms[1].NextTimestamp.IsZero() {
		t.Errorf("one-off alarm decoded incorrectly: %+v", alarms[1])
	}
}

func TestDisablingAnAlarmSendsTheWholeObjectWithoutComputedFields(t *testing.T) {
	c, api := newTestClient(t, alarmServer)

	if err := c.SetAlarmEnabled(context.Background(), SideMine, "alarm-weekday", false); err != nil {
		t.Fatalf("SetAlarmEnabled: %v", err)
	}
	calls := api.calls(http.MethodPut, alarmsWritePath+"/alarm-weekday")
	if len(calls) != 1 {
		t.Fatalf("update endpoint called %d times, want 1", len(calls))
	}
	body := calls[0].Body
	if body["enabled"] != false || body["time"] != "07:00:00" {
		t.Errorf("unexpected body: %v", body)
	}
	if _, kept := body["futureField"]; !kept {
		t.Error("fields clim8 does not model were dropped from the update")
	}
	for _, key := range alarmComputedFields {
		if _, present := body[key]; present {
			t.Errorf("server-computed field %q was sent back", key)
		}
	}
}

func TestUpdatingAnUnknownAlarmFailsWithoutWriting(t *testing.T) {
	c, api := newTestClient(t, alarmServer)

	if err := c.SetAlarmEnabled(context.Background(), SideMine, "nope", true); err == nil {
		t.Fatal("expected an error for an unknown alarm id")
	}
	for _, r := range api.requests {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected write: %s %s", r.Method, r.Path)
		}
	}
}

func TestCreateAlarmValidatesAndFormatsTheRequest(t *testing.T) {
	c, api := newTestClient(t, alarmServer)

	bad := []AlarmOptions{
		{Time: "7am"},
		{Time: "25:00"},
		{Time: "07:30", VibrationLevel: 101},
		{Time: "07:30", ThermalLevel: -101},
	}
	for _, opts := range bad {
		if err := c.CreateAlarm(context.Background(), SideMine, opts); err == nil {
			t.Errorf("CreateAlarm(%+v): expected an error", opts)
		}
	}
	if len(api.requests) != 0 {
		t.Fatalf("invalid alarms still sent %d requests", len(api.requests))
	}

	opts := AlarmOptions{Time: "7:30", VibrationLevel: 0, ThermalLevel: 20}
	if err := c.CreateAlarm(context.Background(), SideMine, opts); err != nil {
		t.Fatalf("CreateAlarm: %v", err)
	}
	calls := api.calls(http.MethodPost, alarmsWritePath)
	if len(calls) != 1 || calls[0].Body["time"] != "07:30:00" {
		t.Fatalf("unexpected create requests: %+v", calls)
	}
	vibration, _ := calls[0].Body["vibration"].(map[string]any)
	thermal, _ := calls[0].Body["thermal"].(map[string]any)
	if vibration["enabled"] != false || thermal["level"] != float64(20) {
		t.Errorf("unexpected create body: %v", calls[0].Body)
	}
}

func TestDismissWithoutAnIDNeedsARingingAlarm(t *testing.T) {
	c, api := newTestClient(t, alarmServer)

	if err := c.DismissAlarm(context.Background(), SideMine, ""); err == nil {
		t.Fatal("expected an error when no alarm is ringing")
	}
	if calls := api.calls(http.MethodPut, alarmsWritePath+"/alarm-weekday/dismiss"); len(calls) != 0 {
		t.Fatal("dismissed an alarm that was not ringing")
	}
}

func TestSnoozeTargetsTheRingingAlarm(t *testing.T) {
	ringing := strings.Replace(alarmsJSON, `"snoozing":false,"snoozedUntil"`,
		`"snoozing":true,"snoozedUntil"`, 1)
	c, api := newTestClient(t, func(r recordedRequest) fakeResponse {
		if r.Method == http.MethodGet {
			return ok(ringing)
		}
		return ok(`{}`)
	})

	if err := c.SnoozeAlarm(context.Background(), SideMine, "", 9); err != nil {
		t.Fatalf("SnoozeAlarm: %v", err)
	}
	calls := api.calls(http.MethodPut, alarmsWritePath+"/alarm-weekday/snooze")
	if len(calls) != 1 || calls[0].Body["snoozeMinutes"] != float64(9) ||
		calls[0].Body["ignoreDeviceErrors"] != false {
		t.Fatalf("unexpected snooze requests: %+v", calls)
	}
	if err := c.SnoozeAlarm(context.Background(), SideMine, "alarm-weekday", 0); err == nil {
		t.Error("expected an error for a zero-minute snooze")
	}
}

func TestAlarmIsRingingOnlyInsideItsWindow(t *testing.T) {
	start := time.Date(2026, 9, 22, 12, 55, 0, 0, time.UTC)
	alarm := Alarm{StartTimestamp: start, EndTimestamp: start.Add(35 * time.Minute)}

	tests := map[string]struct {
		now  time.Time
		want bool
	}{
		"before the window": {start.Add(-time.Second), false},
		"at the start":      {start, true},
		"at the end":        {start.Add(35 * time.Minute), true},
		"after the window":  {start.Add(36 * time.Minute), false},
	}
	for name, tc := range tests {
		if got := alarm.IsRinging(tc.now); got != tc.want {
			t.Errorf("%s: IsRinging = %t, want %t", name, got, tc.want)
		}
	}
	if (Alarm{}).IsRinging(start) {
		t.Error("an alarm with no window must not count as ringing")
	}
}

func TestAlarmCommandsRejectBothSides(t *testing.T) {
	c, api := newTestClient(t, alarmServer)
	if _, err := c.ListAlarms(context.Background(), SideBoth); err == nil {
		t.Fatal("expected an error: alarms belong to one user")
	}
	if len(api.calls(http.MethodGet, alarmsListPath)) != 0 {
		t.Fatal("listed alarms despite an invalid side")
	}
}
