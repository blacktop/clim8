package eightsleep

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func podPath(userID string) string  { return "/app/v1/users/" + userID + "/temperature/pod" }
func tempPath(userID string) string { return "/app/v1/users/" + userID + "/temperature" }

func podState(stateType string, level int) string {
	return `{"devices":[{"device":{"deviceId":"` + testDevice + `"},"currentLevel":` +
		strconv.Itoa(level) + `,"currentState":{"type":"` + stateType + `"}}]}`
}

func TestSideUserIDsSurviveAwayMode(t *testing.T) {
	tests := []struct {
		name                string
		left, right         string
		awayLeft, awayRight string
		wantLeft, wantRight string
		leftAway, rightAway bool
	}{
		{name: "both present", left: "a", right: "b", awayLeft: "a", awayRight: "b",
			wantLeft: "a", wantRight: "b"},
		{name: "no awaySides reported", left: "a", right: "b", wantLeft: "a", wantRight: "b"},
		{name: "left away collapses slots onto the right user",
			left: "b", right: "b", awayLeft: "a", awayRight: "b",
			wantLeft: "a", wantRight: "b", leftAway: true},
		{name: "right away collapses slots onto the left user",
			left: "a", right: "a", awayLeft: "a", awayRight: "b",
			wantLeft: "a", wantRight: "b", rightAway: true},
		{name: "everyone away blanks the slots", awayLeft: "a", awayRight: "b",
			wantLeft: "a", wantRight: "b", leftAway: true, rightAway: true},
		{name: "solo sleeper owns both sides", left: "a", right: "a", awayLeft: "a", awayRight: "a",
			wantLeft: "a", wantRight: "a"},
		{name: "only the left side is assigned", left: "a", wantLeft: "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Device{LeftUserID: tc.left, RightUserID: tc.right}
			d.AwaySides.LeftUserID, d.AwaySides.RightUserID = tc.awayLeft, tc.awayRight

			left, right := sideUserIDs(d)
			if left != tc.wantLeft || right != tc.wantRight {
				t.Errorf("sideUserIDs = (%q, %q), want (%q, %q)", left, right, tc.wantLeft, tc.wantRight)
			}
			if got := sideIsAway(d.LeftUserID, d.AwaySides.LeftUserID); got != tc.leftAway {
				t.Errorf("left away = %t, want %t", got, tc.leftAway)
			}
			if got := sideIsAway(d.RightUserID, d.AwaySides.RightUserID); got != tc.rightAway {
				t.Errorf("right away = %t, want %t", got, tc.rightAway)
			}
		})
	}
}

func TestSideSelectionResolvesToUsers(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })

	tests := []struct {
		side Side
		want []string
	}{
		{SideMine, []string{testMe}},
		{SideLeft, []string{testMe}},
		{SideRight, []string{testPartner}},
		{SideBoth, []string{testMe, testPartner}},
	}
	for _, tc := range tests {
		got, err := c.userIDs(tc.side)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("userIDs(%q) = %v, %v; want %v", tc.side, got, err, tc.want)
		}
	}

	c.devices[0].RightUserID = ""
	if _, err := c.userIDs(SideRight); err == nil {
		t.Error("expected an error for a side with no user assigned")
	}
	if _, err := c.userIDs(SideBoth); err == nil {
		t.Error("expected an error for both sides when only one is assigned")
	}
	if _, err := c.userID(SideBoth); err == nil {
		t.Error("expected an error when a single-user command is given both sides")
	}
}

func TestParsersRejectBadInput(t *testing.T) {
	for _, s := range []string{"", "left", "RIGHT", " both "} {
		if _, err := ParseSide(s); err != nil {
			t.Errorf("ParseSide(%q): %v", s, err)
		}
	}
	for _, s := range []string{"middle", "solo", "l"} {
		if _, err := ParseSide(s); err == nil {
			t.Errorf("ParseSide(%q): expected an error", s)
		}
	}
	for _, s := range []string{"", "68", "F", "sixtyF", "68.5F", "68f"} {
		if _, _, err := ParseTemperature(s); err == nil {
			t.Errorf("ParseTemperature(%q): expected an error", s)
		}
	}
	if stage, err := ParseSleepStage("Final"); err != nil || stage != StageFinal {
		t.Errorf("ParseSleepStage(Final) = %q, %v", stage, err)
	}
	if _, err := ParseSleepStage("nap"); err == nil {
		t.Error("ParseSleepStage(nap): expected an error")
	}
}

func TestPowerTargetsEverySelectedSide(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse { return ok(podState("smart", 0)) })

	if err := c.TurnOn(context.Background(), SideBoth); err != nil {
		t.Fatalf("TurnOn: %v", err)
	}
	for _, userID := range []string{testMe, testPartner} {
		calls := api.calls(http.MethodPut, podPath(userID))
		if len(calls) != 1 {
			t.Fatalf("user %s received %d power requests, want 1", userID, len(calls))
		}
		state, _ := calls[0].Body["currentState"].(map[string]any)
		if state["type"] != "smart" {
			t.Errorf("user %s body = %v, want currentState.type smart", userID, calls[0].Body)
		}
	}
}

func TestPowerFailsWhenThePodReportsTheOppositeState(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse { return ok(podState("smart", 0)) })
	if err := c.TurnOff(context.Background(), SideMine); err == nil {
		t.Error("TurnOff: expected an error when the pod still reports smart")
	}

	c, _ = newTestClient(t, func(recordedRequest) fakeResponse { return ok(podState("off", 0)) })
	if err := c.TurnOn(context.Background(), SideMine); err == nil {
		t.Error("TurnOn: expected an error when the pod still reports off")
	}

	c, _ = newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{"devices":[]}`) })
	if err := c.TurnOn(context.Background(), SideMine); err == nil {
		t.Error("TurnOn: expected an error for a response with no devices")
	}
}

func TestSetTemperatureWithHoldSendsATimedLevel(t *testing.T) {
	level := TempToHeatingLevel(68, Fahrenheit)
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return ok(podState("smart", level))
	})

	if err := c.SetTemperature(context.Background(), SideRight, "68F", 90*time.Minute); err != nil {
		t.Fatalf("SetTemperature: %v", err)
	}
	levelCalls := api.calls(http.MethodPut, podPath(testPartner))
	if len(levelCalls) != 1 || levelCalls[0].Body["currentLevel"] != float64(level) {
		t.Fatalf("unexpected level requests: %+v", levelCalls)
	}
	holdCalls := api.calls(http.MethodPut, tempPath(testPartner))
	if len(holdCalls) != 1 {
		t.Fatalf("hold endpoint called %d times, want 1", len(holdCalls))
	}
	timed, _ := holdCalls[0].Body["timeBased"].(map[string]any)
	if timed["level"] != float64(level) || timed["durationSeconds"] != float64(5400) {
		t.Fatalf("unexpected hold body: %v", holdCalls[0].Body)
	}
}

func TestSetTemperatureWithoutHoldSendsNoTimedLevel(t *testing.T) {
	level := TempToHeatingLevel(20, Celsius)
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return ok(podState("smart", level))
	})

	if err := c.SetTemperature(context.Background(), SideMine, "20C", 0); err != nil {
		t.Fatalf("SetTemperature: %v", err)
	}
	if calls := api.calls(http.MethodPut, tempPath(testMe)); len(calls) != 0 {
		t.Fatalf("hold endpoint called %d times, want 0", len(calls))
	}
}

func TestSetTemperatureRejectsBadInputAndIgnoredLevels(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return ok(podState("smart", 100))
	})

	if err := c.SetTemperature(context.Background(), SideMine, "warm", 0); err == nil {
		t.Error("expected an error for a malformed temperature")
	}
	if err := c.SetTemperature(context.Background(), SideMine, "68F", -time.Minute); err == nil {
		t.Error("expected an error for a negative hold")
	}
	if len(api.requests) != 0 {
		t.Fatalf("invalid input still sent %d requests", len(api.requests))
	}
	if err := c.SetTemperature(context.Background(), SideMine, "60F", 0); err == nil {
		t.Error("expected an error when the pod reports a different level than requested")
	}
}

func TestStageTemperatureKeepsTheOtherStages(t *testing.T) {
	c, api := newTestClient(t, func(r recordedRequest) fakeResponse {
		if r.Method == http.MethodGet {
			return ok(`{"smart":{"bedTimeLevel":-10,"initialSleepLevel":-20,"finalSleepLevel":5}}`)
		}
		return ok(`{}`)
	})

	if err := c.SetStageTemperature(context.Background(), SideMine, StageFinal, "70F"); err != nil {
		t.Fatalf("SetStageTemperature: %v", err)
	}
	calls := api.calls(http.MethodPut, tempPath(testMe))
	if len(calls) != 1 {
		t.Fatalf("temperature endpoint received %d PUTs, want 1", len(calls))
	}
	smart, _ := calls[0].Body["smart"].(map[string]any)
	want := float64(TempToHeatingLevel(70, Fahrenheit))
	if smart["finalSleepLevel"] != want || smart["bedTimeLevel"] != float64(-10) ||
		smart["initialSleepLevel"] != float64(-20) {
		t.Fatalf("unexpected Autopilot levels: %v", smart)
	}
}

func TestStageTemperatureFailsWithoutAutopilotLevels(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })
	if err := c.SetStageTemperature(context.Background(), SideMine, StageBedtime, "70F"); err == nil {
		t.Fatal("expected an error when the response has no smart levels")
	}
}

func TestAwayModeBackdatesTheChange(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })

	if err := c.SetAway(context.Background(), SideRight, true); err != nil {
		t.Fatalf("SetAway(true): %v", err)
	}
	if err := c.SetAway(context.Background(), SideRight, false); err != nil {
		t.Fatalf("SetAway(false): %v", err)
	}
	calls := api.calls(http.MethodPut, "/app/v1/users/"+testPartner+"/away-mode")
	if len(calls) != 2 {
		t.Fatalf("away endpoint called %d times, want 2", len(calls))
	}
	for i, key := range []string{"start", "end"} {
		period, _ := calls[i].Body["awayPeriod"].(map[string]any)
		stamp, _ := period[key].(string)
		when, err := time.Parse("2006-01-02T15:04:05.000Z", stamp)
		if err != nil || !when.Before(time.Now()) || len(period) != 1 {
			t.Errorf("call %d: awayPeriod = %v, want a single past %q timestamp", i, period, key)
		}
	}
}

func TestStatusReportsAwaySidesAndUnits(t *testing.T) {
	c, _ := newTestClient(t, func(recordedRequest) fakeResponse { return ok(`{}`) })
	c.me.DisplaySettings.MeasurementSystem = "metric"
	d := &c.devices[0]
	d.LeftUserID, d.RightUserID = testPartner, testPartner
	d.AwaySides.LeftUserID, d.AwaySides.RightUserID = testMe, testPartner
	d.RightKelvin.Active = true
	d.RightHeatingLevel = TempToHeatingLevel(20, Celsius)

	pods := c.Status()
	if len(pods) != 1 || len(pods[0].Sides) != 2 {
		t.Fatalf("unexpected status shape: %+v", pods)
	}
	left, right := pods[0].Sides[0], pods[0].Sides[1]
	if !left.Away || left.UserID != testMe {
		t.Errorf("left side = %+v, want away and owned by %s", left, testMe)
	}
	if right.Away || !right.Active || right.Temperature != 20 || pods[0].Unit != Celsius {
		t.Errorf("right side = %+v (unit %s), want active at 20C", right, pods[0].Unit)
	}
}

func TestSleepDaysQueriesTheDeviceTimezoneAndToleratesAwayNights(t *testing.T) {
	c, api := newTestClient(t, func(recordedRequest) fakeResponse {
		return ok(`{"days":[{"day":"2026-09-20","score":87,"sleepDuration":27000,
			"presenceStart":null,"sleepQualityScore":{"hrv":{"average":52.5}}}]}`)
	})
	date := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

	days, err := c.SleepDays(context.Background(), SideMine, date, date)
	if err != nil {
		t.Fatalf("SleepDays: %v", err)
	}
	if len(days) != 1 || days[0].Score != 87 || days[0].Quality.HRV.Average != 52.5 ||
		!days[0].PresenceStart.IsZero() {
		t.Fatalf("unexpected days: %+v", days)
	}
	calls := api.calls(http.MethodGet, "/client/users/"+testMe+"/trends")
	if len(calls) != 1 {
		t.Fatalf("trends endpoint called %d times, want 1", len(calls))
	}
	for _, want := range []string{"tz=America%2FDenver", "from=2026-09-20", "to=2026-09-20"} {
		if !strings.Contains(calls[0].Query, want) {
			t.Errorf("query %q is missing %q", calls[0].Query, want)
		}
	}

	if _, err := c.SleepDays(context.Background(), SideMine, date, date.AddDate(0, 0, -1)); err == nil {
		t.Error("expected an error for a reversed date range")
	}
}
