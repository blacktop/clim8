package eightsleep

import "time"

type Token struct {
	Bearer     string
	Expiration time.Time
	MainID     string
}

type UnitOfTemperature string

const (
	Celsius    UnitOfTemperature = "c"
	Fahrenheit UnitOfTemperature = "f"
)

type Profile struct {
	ID                      string    `json:"userId"`
	Email                   string    `json:"email"`
	FirstName               string    `json:"firstName"`
	LastName                string    `json:"lastName"`
	Gender                  string    `json:"gender"`
	TempPreference          string    `json:"tempPreference"`
	TempPreferenceUpdatedAt time.Time `json:"tempPreferenceUpdatedAt"`
	Dob                     time.Time `json:"dob"`
	Zip                     int       `json:"zip"`
	EmailVerified           bool      `json:"emailVerified"`
	SharingMetricsTo        []any     `json:"sharingMetricsTo"`
	SharingMetricsFrom      []any     `json:"sharingMetricsFrom"`
	Notifications           struct {
		WeeklyReportEmail         bool `json:"weeklyReportEmail"`
		SessionProcessed          bool `json:"sessionProcessed"`
		TemperatureRecommendation bool `json:"temperatureRecommendation"`
		HealthInsight             bool `json:"healthInsight"`
		SleepInsight              bool `json:"sleepInsight"`
		MarketingUpdates          bool `json:"marketingUpdates"`
		BedtimeReminder           bool `json:"bedtimeReminder"`
		AlarmWakeupPush           bool `json:"alarmWakeupPush"`
	} `json:"notifications"`
	DisplaySettings struct {
		MeasurementSystem   string `json:"measurementSystem"`
		UseRealTemperatures bool   `json:"useRealTemperatures"`
	} `json:"displaySettings"`
	CreatedAt            time.Time `json:"createdAt"`
	ExperimentalFeatures bool      `json:"experimentalFeatures"`
	AutopilotEnabled     bool      `json:"autopilotEnabled"`
	LastReset            time.Time `json:"lastReset"`
	NextReset            time.Time `json:"nextReset"`
	SleepTracking        struct {
		EnabledSince time.Time `json:"enabledSince"`
	} `json:"sleepTracking"`
	Features      []string `json:"features"`
	CurrentDevice struct {
		ID             string `json:"id"`
		Side           string `json:"side"`
		TimeZone       string `json:"timeZone"`
		Specialization string `json:"specialization"`
	} `json:"currentDevice"`
	HotelGuest bool     `json:"hotelGuest"`
	Devices    []string `json:"devices"`
}

type Device struct {
	ID                     string `json:"deviceId"`
	OwnerID                string `json:"ownerId"`
	LeftUserID             string `json:"leftUserId"`
	LeftHeatingLevel       int    `json:"leftHeatingLevel"`
	LeftTargetHeatingLevel int    `json:"leftTargetHeatingLevel"`
	LeftNowHeating         bool   `json:"leftNowHeating"`
	LeftHeatingDuration    int    `json:"leftHeatingDuration"`
	LeftSchedule           struct {
		DaysUTC struct {
			Sunday    bool `json:"sunday"`
			Monday    bool `json:"monday"`
			Tuesday   bool `json:"tuesday"`
			Wednesday bool `json:"wednesday"`
			Thursday  bool `json:"thursday"`
			Friday    bool `json:"friday"`
			Saturday  bool `json:"saturday"`
		} `json:"daysUTC"`
		Enabled bool `json:"enabled"`
	} `json:"leftSchedule"`
	RightUserID             string `json:"rightUserId"`
	RightHeatingLevel       int    `json:"rightHeatingLevel"`
	RightTargetHeatingLevel int    `json:"rightTargetHeatingLevel"`
	RightNowHeating         bool   `json:"rightNowHeating"`
	RightHeatingDuration    int    `json:"rightHeatingDuration"`
	RightSchedule           struct {
		DaysUTC struct {
			Sunday    bool `json:"sunday"`
			Monday    bool `json:"monday"`
			Tuesday   bool `json:"tuesday"`
			Wednesday bool `json:"wednesday"`
			Thursday  bool `json:"thursday"`
			Friday    bool `json:"friday"`
			Saturday  bool `json:"saturday"`
		} `json:"daysUTC"`
		Enabled bool `json:"enabled"`
	} `json:"rightSchedule"`
	Priming            bool      `json:"priming"`
	LastLowWater       time.Time `json:"lastLowWater"`
	NeedsPriming       bool      `json:"needsPriming"`
	HasWater           bool      `json:"hasWater"`
	LedBrightnessLevel int       `json:"ledBrightnessLevel"`
	SensorInfo         struct {
		Label         string    `json:"label"`
		PartNumber    string    `json:"partNumber"`
		Sku           string    `json:"sku"`
		HwRevision    string    `json:"hwRevision"`
		SerialNumber  string    `json:"serialNumber"`
		LastConnected time.Time `json:"lastConnected"`
		SkuName       string    `json:"skuName"`
		Model         string    `json:"model"`
		Version       int       `json:"version"`
		Connected     bool      `json:"connected"`
	} `json:"sensorInfo"`
	Sensors []struct {
		Label         string    `json:"label"`
		PartNumber    string    `json:"partNumber"`
		Sku           string    `json:"sku"`
		HwRevision    string    `json:"hwRevision"`
		SerialNumber  string    `json:"serialNumber"`
		LastConnected time.Time `json:"lastConnected"`
		SkuName       string    `json:"skuName"`
		Model         string    `json:"model"`
		Version       int       `json:"version"`
		Connected     bool      `json:"connected"`
	} `json:"sensors"`
	ExpectedPeripherals []struct {
		PeripheralType string `json:"peripheralType"`
	} `json:"expectedPeripherals"`
	HubInfo      string `json:"hubInfo"`
	Timezone     string `json:"timezone"`
	MattressInfo struct {
		FirstUsedDate any `json:"firstUsedDate"`
		EightMattress any `json:"eightMattress"`
		Brand         any `json:"brand"`
	} `json:"mattressInfo"`
	FirmwareCommit          string    `json:"firmwareCommit"`
	FirmwareVersion         string    `json:"firmwareVersion"`
	FirmwareUpdated         bool      `json:"firmwareUpdated"`
	FirmwareUpdating        bool      `json:"firmwareUpdating"`
	LastFirmwareUpdateStart time.Time `json:"lastFirmwareUpdateStart"`
	LastHeard               time.Time `json:"lastHeard"`
	Online                  bool      `json:"online"`
	EncasementType          any       `json:"encasementType"`
	LeftKelvin              struct {
		TargetLevels       []int  `json:"targetLevels"`
		ScheduleProfiles   []any  `json:"scheduleProfiles"`
		Alarms             []any  `json:"alarms"`
		Level              int    `json:"level"`
		CurrentTargetLevel int    `json:"currentTargetLevel"`
		Active             bool   `json:"active"`
		CurrentActivity    string `json:"currentActivity"`
	} `json:"leftKelvin"`
	RightKelvin struct {
		TargetLevels     []int `json:"targetLevels"`
		Alarms           []any `json:"alarms"`
		ScheduleProfiles []struct {
			Enabled        bool   `json:"enabled"`
			StartLocalTime string `json:"startLocalTime"`
			WeekDays       struct {
				Monday    bool `json:"monday"`
				Tuesday   bool `json:"tuesday"`
				Wednesday bool `json:"wednesday"`
				Thursday  bool `json:"thursday"`
				Friday    bool `json:"friday"`
				Saturday  bool `json:"saturday"`
				Sunday    bool `json:"sunday"`
			} `json:"weekDays"`
		} `json:"scheduleProfiles"`
		Phases             []any  `json:"phases"`
		Level              int    `json:"level"`
		CurrentTargetLevel int    `json:"currentTargetLevel"`
		Active             bool   `json:"active"`
		CurrentActivity    string `json:"currentActivity"`
	} `json:"rightKelvin"`
	Features                   []string `json:"features"`
	LeftUserInvitationPending  bool     `json:"leftUserInvitationPending"`
	RightUserInvitationPending bool     `json:"rightUserInvitationPending"`
	ModelString                string   `json:"modelString"`
	HubSerial                  string   `json:"hubSerial"`
	WifiInfo                   struct {
		SignalStrength int       `json:"signalStrength"`
		Ssid           string    `json:"ssid"`
		IPAddr         string    `json:"ipAddr"`
		MacAddr        string    `json:"macAddr"`
		AsOf           time.Time `json:"asOf"`
	} `json:"wifiInfo"`
	AwaySides struct {
		LeftUserID  string `json:"leftUserId"`
		RightUserID string `json:"rightUserId"`
	} `json:"awaySides"`
	LastPrime              time.Time `json:"lastPrime"`
	IsTemperatureAvailable bool      `json:"isTemperatureAvailable"`
	Deactivated            any       `json:"deactivated"`
}

type TemperatureState struct {
	Devices []struct {
		Device struct {
			DeviceID       string `json:"deviceId"`
			Side           string `json:"side"`
			Specialization string `json:"specialization"`
		} `json:"device"`
		CurrentLevel       int `json:"currentLevel"`
		CurrentDeviceLevel int `json:"currentDeviceLevel"`
		OverrideLevels     struct {
		} `json:"overrideLevels"`
		CurrentState struct {
			Type     string    `json:"type"`
			Until    time.Time `json:"until"`
			Instance struct {
				Timestamp time.Time `json:"timestamp"`
			} `json:"instance"`
		} `json:"currentState"`
		Smart struct {
			BedTimeLevel      int `json:"bedTimeLevel"`
			InitialSleepLevel int `json:"initialSleepLevel"`
			FinalSleepLevel   int `json:"finalSleepLevel"`
		} `json:"smart"`
	} `json:"devices"`
	TemperatureSettings []struct {
		Name              string `json:"name"`
		BedTimeLevel      int    `json:"bedTimeLevel"`
		InitialSleepLevel int    `json:"initialSleepLevel"`
		FinalSleepLevel   int    `json:"finalSleepLevel"`
	} `json:"temperatureSettings"`
}
