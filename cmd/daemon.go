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
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ScheduleItem represents a scheduled action
type ScheduleItem struct {
	Time        string `yaml:"time"`
	Action      string `yaml:"action"`
	Temperature string `yaml:"temperature,omitempty"`
}

// ScheduleConfig represents the schedule configuration
type ScheduleConfig struct {
	Schedule []ScheduleItem `yaml:"schedule"`
}

// daemonCmd represents the daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run Eight Sleep scheduler daemon",
	Long: `Run a background daemon that executes Eight Sleep commands on a schedule.

The daemon reads a schedule from the config file and executes actions at specified times.
Example config:

schedule:
  - time: "22:00"
    action: "on"
  - time: "22:15"
    action: "temp"
    temperature: "68"
  - time: "06:00"
    action: "off"
`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if viper.GetBool("verbose") {
			logger.SetLevel(log.DebugLevel)
		}

		// Check config file security
		if err := checkConfigSecurity(); err != nil {
			logger.Warn("Config file security issue", "warning", err)
		}

		// Create PID file to prevent multiple instances
		pidFile, err := createPidFile()
		if err != nil {
			return fmt.Errorf("failed to create PID file: %w", err)
		}
		defer removePidFile(pidFile)

		// Parse schedule from config
		var config ScheduleConfig
		if err := viper.Unmarshal(&config); err != nil {
			return fmt.Errorf("failed to parse schedule config: %w", err)
		}

		if len(config.Schedule) == 0 {
			return fmt.Errorf("no schedule items found in config")
		}

		// Validate schedule items
		for i, item := range config.Schedule {
			if err := validateScheduleItem(item); err != nil {
				return fmt.Errorf("invalid schedule item %d: %w", i, err)
			}
		}

		logger.Info("Starting Eight Sleep scheduler daemon", "items", len(config.Schedule))

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			logger.Info("Received shutdown signal, stopping daemon...")
			cancel()
		}()

		// Create a single Eight Sleep client for the daemon lifetime
		cli, err := eightsleep.NewClient(
			viper.GetString("email"),
			viper.GetString("password"),
			viper.GetString("daemon.timezone"),
		)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer cli.Stop()

		if err := cli.Start(ctx); err != nil {
			return fmt.Errorf("failed to start client: %w", err)
		}

		// Run the scheduler
		return runScheduler(ctx, cli, config.Schedule)
	},
}

func checkConfigSecurity() error {
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		return fmt.Errorf("no config file in use")
	}

	info, err := os.Stat(configFile)
	if err != nil {
		return fmt.Errorf("cannot stat config file: %w", err)
	}

	// Check if file is readable by others
	mode := info.Mode()
	if mode&0044 != 0 {
		return fmt.Errorf("config file %s is readable by others (permissions: %o), should be 600", configFile, mode&0777)
	}

	return nil
}

func createPidFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	pidFile := filepath.Join(home, ".config", "clim8", "daemon.pid")

	// Check if PID file exists and process is running
	if data, err := os.ReadFile(pidFile); err == nil {
		if pid := strings.TrimSpace(string(data)); pid != "" {
			// Check if process is still running (Unix-specific)
			if _, err := os.Stat("/proc/" + pid); err == nil {
				return "", fmt.Errorf("daemon already running with PID %s", pid)
			}
		}
	}

	// Write current PID
	pid := fmt.Sprintf("%d", os.Getpid())
	if err := os.WriteFile(pidFile, []byte(pid), 0600); err != nil {
		return "", fmt.Errorf("failed to write PID file: %w", err)
	}

	return pidFile, nil
}

func removePidFile(pidFile string) {
	if pidFile != "" {
		os.Remove(pidFile)
	}
}

func validateTemperature(temp string) error {
	if len(temp) < 2 {
		return fmt.Errorf("temperature must be a number followed by F or C")
	}

	// Get the unit (last character)
	unit := temp[len(temp)-1:]
	if unit != "F" && unit != "C" {
		return fmt.Errorf("temperature must end with F (Fahrenheit) or C (Celsius)")
	}

	// Get the numeric part (everything except the last character)
	numPart := temp[:len(temp)-1]
	if _, err := strconv.ParseFloat(numPart, 64); err != nil {
		return fmt.Errorf("temperature numeric part must be a valid number")
	}

	return nil
}

func validateScheduleItem(item ScheduleItem) error {
	// Validate time format (HH:MM)
	if _, err := parseTime(item.Time); err != nil {
		return fmt.Errorf("invalid time format '%s': %w", item.Time, err)
	}

	// Validate action
	switch item.Action {
	case "on", "off":
		// No additional validation needed
	case "temp":
		if item.Temperature == "" {
			return fmt.Errorf("temperature required for temp action")
		}
		// Validate temperature format (number + F/C)
		if err := validateTemperature(item.Temperature); err != nil {
			return fmt.Errorf("invalid temperature '%s': %w", item.Temperature, err)
		}
	default:
		return fmt.Errorf("unknown action '%s'", item.Action)
	}

	return nil
}

func parseTime(timeStr string) (time.Time, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("time must be in HH:MM format")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, fmt.Errorf("invalid hour")
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid minute")
	}

	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location()), nil
}

func logUpcomingSchedule(schedule []ScheduleItem) {
	logger.Info("Loaded schedule:")
	for i, item := range schedule {
		if item.Temperature != "" {
			logger.Info(fmt.Sprintf("  %d. %s - %s", i+1, item.Time, item.Action),
				"temperature", item.Temperature)
		} else {
			logger.Info(fmt.Sprintf("  %d. %s - %s", i+1, item.Time, item.Action))
		}
	}
}

func runScheduler(ctx context.Context, cli *eightsleep.Client, schedule []ScheduleItem) error {
	// Track executed items to prevent duplicates
	executed := make(map[string]bool)
	var lastDay int

	// Log upcoming schedule
	logUpcomingSchedule(schedule)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Separate interval for state sync to reduce API calls
	syncInterval := viper.GetDuration("daemon.sync-interval")
	if syncInterval == 0 {
		syncInterval = 5 * time.Minute
	}
	syncTicker := time.NewTicker(syncInterval)
	defer syncTicker.Stop()

	logger.Info("Scheduler started", "sync-interval", syncInterval)

	// Immediate sync on startup to catch up after sleep/wake
	if viper.GetBool("daemon.sync-state") {
		if err := checkAndSyncDeviceState(ctx, cli, schedule); err != nil {
			logger.Warn("Failed initial state sync", "err", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("Scheduler stopped")
			return nil
		case <-syncTicker.C:
			// Refresh device cache periodically to avoid stale data
			if err := cli.RefreshDevices(ctx); err != nil {
				logger.Warn("Failed to refresh device cache", "err", err)
			}

			// Check and sync device state periodically
			if viper.GetBool("daemon.sync-state") {
				if err := checkAndSyncDeviceState(ctx, cli, schedule); err != nil {
					logger.Warn("Failed to check/sync device state", "err", err)
				}
			}
		case <-ticker.C:
			now := time.Now()

			// Reset execution tracking at start of new day
			if now.Day() != lastDay {
				executed = make(map[string]bool)
				lastDay = now.Day()
				logger.Info("New day started, reset execution tracking", "date", now.Format("2006-01-02"))
			}

			if err := processSchedule(ctx, cli, schedule, executed); err != nil {
				logger.Error("Error processing schedule", "err", err)
			}
		}
	}
}

func processSchedule(ctx context.Context, cli *eightsleep.Client, schedule []ScheduleItem, executed map[string]bool) error {
	now := time.Now()

	// Find items that should run now (within the last minute)
	for _, item := range schedule {
		scheduledTime, err := parseTime(item.Time)
		if err != nil {
			continue
		}

		// Create unique key for this execution
		execKey := fmt.Sprintf("%s-%s-%s",
			now.Format("2006-01-02"),
			item.Time,
			item.Action)

		// Skip if already executed today
		if executed[execKey] {
			continue
		}

		// If scheduled time is today and within the last minute
		if scheduledTime.Day() == now.Day() &&
			scheduledTime.Month() == now.Month() &&
			scheduledTime.Year() == now.Year() {

			timeDiff := now.Sub(scheduledTime)
			if timeDiff >= 0 && timeDiff < time.Minute {
				if item.Temperature != "" {
					logger.Info("Executing scheduled action",
						"time", item.Time,
						"action", item.Action,
						"temperature", item.Temperature)
				} else {
					logger.Info("Executing scheduled action",
						"time", item.Time,
						"action", item.Action)
				}

				if err := executeAction(ctx, cli, item); err != nil {
					logger.Error("Failed to execute action",
						"action", item.Action,
						"err", err)
				} else {
					// Mark as executed
					executed[execKey] = true
				}
			}
		}
	}

	return nil
}

func executeAction(ctx context.Context, cli *eightsleep.Client, item ScheduleItem) error {
	// Check if this is a dry run
	if viper.GetBool("daemon.dry-run") {
		if item.Temperature != "" {
			logger.Info("DRY RUN - Would execute action",
				"action", item.Action,
				"temperature", item.Temperature)
		} else {
			logger.Info("DRY RUN - Would execute action",
				"action", item.Action)
		}
		return nil
	}

	switch item.Action {
	case "on":
		if err := cli.TurnOn(ctx); err != nil {
			return fmt.Errorf("failed to turn on: %w", err)
		}
		logger.Info("Device turned ON")

	case "off":
		if err := cli.TurnOff(ctx); err != nil {
			return fmt.Errorf("failed to turn off: %w", err)
		}
		logger.Info("Device turned OFF")

	case "temp":
		// Turn on first, then set temperature
		// If SetTemperature fails, attempt rollback by turning off
		if err := cli.TurnOn(ctx); err != nil {
			return fmt.Errorf("failed to turn on before setting temperature: %w", err)
		}

		if err := cli.SetTemperature(ctx, item.Temperature); err != nil {
			// Attempt rollback - best effort, don't fail if rollback fails
			logger.Warn("SetTemperature failed, attempting rollback by turning off", "err", err)
			if rollbackErr := cli.TurnOff(ctx); rollbackErr != nil {
				logger.Error("rollback (TurnOff) also failed", "err", rollbackErr)
			}
			return fmt.Errorf("failed to set temperature: %w", err)
		}
		logger.Info("Temperature set", "temp", item.Temperature)

	default:
		return fmt.Errorf("unknown action: %s", item.Action)
	}

	return nil
}

// getExpectedState determines what the device state should be based on the schedule and current time.
// It handles overnight schedules by checking both today's and yesterday's schedule items.
func getExpectedState(schedule []ScheduleItem, now time.Time) (*ScheduleItem, error) {
	if len(schedule) == 0 {
		return nil, nil
	}

	// Find the most recent schedule item that should have executed
	// We check both today and yesterday to handle overnight schedules
	var mostRecentItem *ScheduleItem
	var mostRecentTime time.Time

	// Check schedule items for today and yesterday
	for daysAgo := 0; daysAgo <= 1; daysAgo++ {
		checkDate := now.AddDate(0, 0, -daysAgo)

		for _, item := range schedule {
			// Parse time gives us today's date with the schedule time
			baseTime, err := parseTime(item.Time)
			if err != nil {
				continue
			}

			// Adjust to the date we're checking (today or yesterday)
			scheduledTime := time.Date(
				checkDate.Year(), checkDate.Month(), checkDate.Day(),
				baseTime.Hour(), baseTime.Minute(), 0, 0, now.Location(),
			)

			// Only consider times in the past
			if scheduledTime.Before(now) {
				if mostRecentItem == nil || scheduledTime.After(mostRecentTime) {
					itemCopy := item
					mostRecentItem = &itemCopy
					mostRecentTime = scheduledTime
				}
			}
		}
	}

	return mostRecentItem, nil
}

// checkAndSyncDeviceState checks if the device is in the expected state and corrects it if needed
func checkAndSyncDeviceState(ctx context.Context, cli *eightsleep.Client, schedule []ScheduleItem) error {
	// Skip if dry run mode
	if viper.GetBool("daemon.dry-run") {
		return nil
	}

	now := time.Now()
	expectedState, err := getExpectedState(schedule, now)
	if err != nil {
		return fmt.Errorf("failed to determine expected state: %w", err)
	}

	// If no expected state (e.g., before first scheduled item today), do nothing
	if expectedState == nil {
		return nil
	}

	// Get current device state
	currentState, err := cli.GetTemperatureState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current temperature state: %w", err)
	}

	// Check if device state matches expected state
	stateMatches, err := deviceStateMatches(currentState, expectedState)
	if err != nil {
		return fmt.Errorf("failed to check device state: %w", err)
	}

	if !stateMatches {
		logger.Info("Device state doesn't match expected state, syncing...",
			"expected_action", expectedState.Action,
			"expected_temp", expectedState.Temperature)

		// Execute the expected action to sync state
		if err := executeAction(ctx, cli, *expectedState); err != nil {
			return fmt.Errorf("failed to sync device state: %w", err)
		}

		logger.Info("Device state synced successfully")
	}

	return nil
}

// deviceStateMatches checks if the current device state matches the expected schedule item
func deviceStateMatches(currentState *eightsleep.TemperatureState, expectedItem *ScheduleItem) (bool, error) {
	if len(currentState.Devices) == 0 {
		return false, fmt.Errorf("no devices found in current state")
	}

	device := currentState.Devices[0] // Assuming single device

	switch expectedItem.Action {
	case "off":
		return device.CurrentState.Type == "off", nil

	case "on":
		return device.CurrentState.Type != "off", nil

	case "temp":
		if device.CurrentState.Type == "off" {
			return false, nil
		}

		if expectedItem.Temperature == "" {
			return false, fmt.Errorf("temperature action requires temperature value")
		}

		expectedTemp, unit, err := eightsleep.ParseTemperature(expectedItem.Temperature)
		if err != nil {
			return false, err
		}

		expectedLevel := eightsleep.TempToHeatingLevel(expectedTemp, unit)

		const tolerance = 2
		diff := device.CurrentLevel - expectedLevel
		if diff < 0 {
			diff = -diff
		}
		return diff <= tolerance, nil

	default:
		return false, fmt.Errorf("unknown action: %s", expectedItem.Action)
	}
}

func init() {
	rootCmd.AddCommand(daemonCmd)

	// Add daemon-specific flags
	daemonCmd.Flags().Bool("dry-run", false, "Show what would be executed without actually running actions")
	daemonCmd.Flags().String("timezone", "America/New_York", "Timezone for schedule execution")
	daemonCmd.Flags().Bool("sync-state", true, "Check and sync device state with schedule after system wake")
	daemonCmd.Flags().Duration("sync-interval", 5*time.Minute, "Interval between device state sync checks")
	viper.BindPFlag("daemon.dry-run", daemonCmd.Flags().Lookup("dry-run"))
	viper.BindPFlag("daemon.timezone", daemonCmd.Flags().Lookup("timezone"))
	viper.BindPFlag("daemon.sync-state", daemonCmd.Flags().Lookup("sync-state"))
	viper.BindPFlag("daemon.sync-interval", daemonCmd.Flags().Lookup("sync-interval"))
}
