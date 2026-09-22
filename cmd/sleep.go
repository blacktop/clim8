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
	"strings"
	"time"

	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// sleepCmd represents the sleep command
var sleepCmd = &cobra.Command{
	Use:     "sleep",
	Short:   "Show sleep data for a day",
	Example: "  clim8 sleep\n  clim8 sleep --date 2026-09-20 --side right",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		dateValue, err := cmd.Flags().GetString("date")
		if err != nil {
			return err
		}
		date := time.Now()
		if dateValue != "" {
			if date, err = time.Parse(time.DateOnly, dateValue); err != nil {
				return fmt.Errorf("invalid date %q (must be YYYY-MM-DD)", dateValue)
			}
		}

		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		days, err := cli.SleepDays(cmd.Context(), side, date, date)
		if err != nil {
			return err
		}
		if viper.GetBool("json") {
			return printJSON(cmd.OutOrStdout(), days, false)
		}
		if len(days) == 0 {
			return fmt.Errorf("no sleep data for %s: the API returned no days for this account "+
				"(sleep reports may need an active Eight Sleep subscription)",
				date.Format(time.DateOnly))
		}
		for _, day := range days {
			if err := renderSleepDay(cmd.OutOrStdout(), day); err != nil {
				return err
			}
		}
		return nil
	},
}

func renderSleepDay(w io.Writer, day eightsleep.SleepDay) error {
	lines := []string{"Sleep for " + day.Day}
	if day.Processing {
		lines = append(lines, "  (still processing - numbers may change)")
	}
	lines = append(lines,
		fmt.Sprintf("  Score:       %.0f (quality %.0f, routine %.0f)",
			day.Score, day.Quality.Total, day.Routine.Total),
		fmt.Sprintf("  In bed:      %s (%s - %s)", formatSeconds(day.PresenceDuration),
			formatTime(day.PresenceStart), formatTime(day.PresenceEnd)),
		fmt.Sprintf("  Asleep:      %s (light %s, deep %s, REM %s)",
			formatSeconds(day.SleepDuration), formatSeconds(day.LightDuration),
			formatSeconds(day.DeepDuration), formatSeconds(day.RemDuration)),
		fmt.Sprintf("  Heart rate:  %.0f bpm", day.Quality.HeartRate.Average),
		fmt.Sprintf("  HRV:         %.0f ms", day.Quality.HRV.Average),
		fmt.Sprintf("  Breathing:   %.1f /min", day.Quality.RespiratoryRate.Average),
		fmt.Sprintf("  Bed / room:  %.1f°C / %.1f°C",
			day.Quality.TempBedC.Average, day.Quality.TempRoomC.Average),
		fmt.Sprintf("  Toss & turn: %.0f", day.TossAndTurns),
	)
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func formatSeconds(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func init() {
	rootCmd.AddCommand(sleepCmd)
	addSideFlag(sleepCmd.Flags())
	sleepCmd.Flags().String("date", "", "Day you woke up, as YYYY-MM-DD (default: today)")
}
