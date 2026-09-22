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
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var weekDayOrder = []string{
	"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
}

// alarmCmd represents the alarm command
var alarmCmd = &cobra.Command{
	Use:   "alarm",
	Short: "Manage Eight Sleep alarms",
}

var alarmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List alarms",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		alarms, err := cli.ListAlarms(cmd.Context(), side)
		if err != nil {
			return err
		}
		if viper.GetBool("json") {
			return printJSON(cmd.OutOrStdout(), alarms, false)
		}
		return renderAlarms(cmd.OutOrStdout(), alarms)
	},
}

var alarmCreateCmd = &cobra.Command{
	Use:     "create <HH:MM>",
	Short:   "Create a one-off alarm",
	Example: "  clim8 alarm create 07:30\n  clim8 alarm create 06:45 --vibration 80 --thermal 20",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		vibration, err := cmd.Flags().GetInt("vibration")
		if err != nil {
			return err
		}
		thermal, err := cmd.Flags().GetInt("thermal")
		if err != nil {
			return err
		}
		opts := eightsleep.AlarmOptions{
			Time:           args[0],
			VibrationLevel: vibration,
			ThermalLevel:   thermal,
		}
		if err := opts.Validate(); err != nil {
			return err
		}

		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		if err := cli.CreateAlarm(cmd.Context(), side, opts); err != nil {
			return err
		}
		logger.Info("Alarm created", "time", args[0])
		return nil
	},
}

func newAlarmEnableCmd(use, short string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			side, err := sideFlag(cmd)
			if err != nil {
				return err
			}
			cli, err := startClient(cmd.Context())
			if err != nil {
				return err
			}
			defer cli.Stop()

			if err := cli.SetAlarmEnabled(cmd.Context(), side, args[0], enabled); err != nil {
				return err
			}
			logger.Info("Alarm updated", "id", args[0], "enabled", enabled)
			return nil
		},
	}
}

var alarmDismissCmd = &cobra.Command{
	Use:   "dismiss [id]",
	Short: "Dismiss an alarm (defaults to the one that is ringing)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		if err := cli.DismissAlarm(cmd.Context(), side, optionalArg(args)); err != nil {
			return err
		}
		logger.Info("Alarm dismissed")
		return nil
	},
}

var alarmSnoozeCmd = &cobra.Command{
	Use:   "snooze [id]",
	Short: "Snooze an alarm (defaults to the one that is ringing)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		minutes, err := cmd.Flags().GetInt("minutes")
		if err != nil {
			return err
		}
		if minutes <= 0 {
			return fmt.Errorf("invalid --minutes %d (must be at least 1)", minutes)
		}
		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		if err := cli.SnoozeAlarm(cmd.Context(), side, optionalArg(args), minutes); err != nil {
			return err
		}
		logger.Info("Alarm snoozed", "minutes", minutes)
		return nil
	},
}

func optionalArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func renderAlarms(w io.Writer, alarms []eightsleep.Alarm) error {
	if len(alarms) == 0 {
		_, err := fmt.Fprintln(w, "No alarms")
		return err
	}
	sort.SliceStable(alarms, func(i, j int) bool { return alarms[i].Time < alarms[j].Time })

	rows := []string{"TIME\tENABLED\tREPEAT\tNEXT\tID"}
	for _, alarm := range alarms {
		rows = append(rows, fmt.Sprintf("%s\t%t\t%s\t%s\t%s",
			alarm.Time, alarm.Enabled, repeatSummary(alarm), formatTime(alarm.NextTimestamp), alarm.ID))
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(rows, "\n")); err != nil {
		return err
	}
	return tw.Flush()
}

func repeatSummary(alarm eightsleep.Alarm) string {
	if !alarm.Repeat.Enabled {
		return "once"
	}
	var days []string
	for _, day := range weekDayOrder {
		if alarm.Repeat.WeekDays[day] {
			days = append(days, day[:3])
		}
	}
	if len(days) == 0 {
		return "once"
	}
	return strings.Join(days, ",")
}

func init() {
	rootCmd.AddCommand(alarmCmd)
	addSideFlag(alarmCmd.PersistentFlags())

	alarmCreateCmd.Flags().Int("vibration", 50, "Vibration power 0-100 (0 disables vibration)")
	alarmCreateCmd.Flags().Int("thermal", 0, "Heating level to wake with, -100 to 100")
	alarmSnoozeCmd.Flags().Int("minutes", 9, "Minutes to snooze for")

	alarmCmd.AddCommand(alarmListCmd)
	alarmCmd.AddCommand(alarmCreateCmd)
	alarmCmd.AddCommand(newAlarmEnableCmd("enable", "Enable an alarm", true))
	alarmCmd.AddCommand(newAlarmEnableCmd("disable", "Disable an alarm", false))
	alarmCmd.AddCommand(alarmDismissCmd)
	alarmCmd.AddCommand(alarmSnoozeCmd)
}
