package eightsleep

import "time"

// SideStatus describes one side of a pod.
type SideStatus struct {
	Side        Side   `json:"side"`
	UserID      string `json:"userId,omitempty"`
	Away        bool   `json:"away"`
	Active      bool   `json:"active"`
	Activity    string `json:"activity,omitempty"`
	Level       int    `json:"level"`
	TargetLevel int    `json:"targetLevel"`
	Temperature int    `json:"temperature"`
	Target      int    `json:"targetTemperature"`
}

// PodStatus describes a pod's hub health and both of its sides.
type PodStatus struct {
	DeviceID         string            `json:"deviceId"`
	Model            string            `json:"model,omitempty"`
	Online           bool              `json:"online"`
	LastHeard        time.Time         `json:"lastHeard"`
	FirmwareVersion  string            `json:"firmwareVersion,omitempty"`
	FirmwareUpdating bool              `json:"firmwareUpdating"`
	WifiSignal       int               `json:"wifiSignal"`
	HasWater         bool              `json:"hasWater"`
	NeedsPriming     bool              `json:"needsPriming"`
	Priming          bool              `json:"priming"`
	LastPrime        time.Time         `json:"lastPrime"`
	LastLowWater     time.Time         `json:"lastLowWater"`
	LedBrightness    int               `json:"ledBrightness"`
	Unit             UnitOfTemperature `json:"unit"`
	Sides            []SideStatus      `json:"sides"`
}

// Status reports every pod on the account from the cached device data.
// Call RefreshDevices first when the cache may be stale.
func (c *Client) Status() []PodStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	unit := Fahrenheit
	if c.me.DisplaySettings.MeasurementSystem == "metric" {
		unit = Celsius
	}

	pods := make([]PodStatus, 0, len(c.devices))
	for _, d := range c.devices {
		left, right := sideUserIDs(d)
		leftSide := SideStatus{
			Side:        SideLeft,
			UserID:      left,
			Away:        sideIsAway(d.LeftUserID, d.AwaySides.LeftUserID),
			Active:      d.LeftKelvin.Active,
			Activity:    d.LeftKelvin.CurrentActivity,
			Level:       d.LeftHeatingLevel,
			TargetLevel: d.LeftTargetHeatingLevel,
		}
		rightSide := SideStatus{
			Side:        SideRight,
			UserID:      right,
			Away:        sideIsAway(d.RightUserID, d.AwaySides.RightUserID),
			Active:      d.RightKelvin.Active,
			Activity:    d.RightKelvin.CurrentActivity,
			Level:       d.RightHeatingLevel,
			TargetLevel: d.RightTargetHeatingLevel,
		}
		sides := []SideStatus{leftSide, rightSide}
		for i := range sides {
			sides[i].Temperature = heatingLevelToTemp(sides[i].Level, unit)
			sides[i].Target = heatingLevelToTemp(sides[i].TargetLevel, unit)
		}
		pods = append(pods, PodStatus{
			DeviceID:         d.ID,
			Model:            d.ModelString,
			Online:           d.Online,
			LastHeard:        d.LastHeard,
			FirmwareVersion:  d.FirmwareVersion,
			FirmwareUpdating: d.FirmwareUpdating,
			WifiSignal:       d.WifiInfo.SignalStrength,
			HasWater:         d.HasWater,
			NeedsPriming:     d.NeedsPriming,
			Priming:          d.Priming,
			LastPrime:        d.LastPrime,
			LastLowWater:     d.LastLowWater,
			LedBrightness:    d.LedBrightnessLevel,
			Unit:             unit,
			Sides:            sides,
		})
	}
	return pods
}
