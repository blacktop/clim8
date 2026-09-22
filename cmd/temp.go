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
	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/spf13/cobra"
)

// tempCmd represents the temp command
var tempCmd = &cobra.Command{
	Use:   "temp <temperature>",
	Short: "Set the temperature of Eight Sleep Pod",
	Long: "Set the temperature of Eight Sleep Pod. " +
		"Temperature must include unit (F for Fahrenheit or C for Celsius).\n\n" +
		"With --for the pod holds the temperature for that long and then stops; a pod with " +
		"nothing else scheduled turns off. With --stage the temperature is saved as an " +
		"Autopilot level instead of being applied now.",
	Example: "  clim8 temp 68F\n  clim8 temp 24C --side both\n" +
		"  clim8 temp 72F --for 2h\n  clim8 temp 66F --stage final",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		temperature := args[0]
		if _, _, err := eightsleep.ParseTemperature(temperature); err != nil {
			return err
		}
		side, err := sideFlag(cmd)
		if err != nil {
			return err
		}
		hold, err := cmd.Flags().GetDuration("for")
		if err != nil {
			return err
		}
		stageName, err := cmd.Flags().GetString("stage")
		if err != nil {
			return err
		}

		var stage eightsleep.SleepStage
		if stageName != "" {
			if stage, err = eightsleep.ParseSleepStage(stageName); err != nil {
				return err
			}
		}

		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		if stage != "" {
			if err := cli.SetStageTemperature(cmd.Context(), side, stage, temperature); err != nil {
				return err
			}
			logger.Info("Autopilot Level Set", "stage", stageName, "temp", temperature)
			return nil
		}

		if err := cli.TurnOn(cmd.Context(), side); err != nil {
			return err
		}

		if err := cli.SetTemperature(cmd.Context(), side, temperature, hold); err != nil {
			// Attempt rollback - best effort, don't fail if rollback fails
			logger.Warn("SetTemperature failed, attempting rollback by turning off", "err", err)
			if rollbackErr := cli.TurnOff(cmd.Context(), side); rollbackErr != nil {
				logger.Error("rollback (TurnOff) also failed", "err", rollbackErr)
			}
			return err
		}
		if hold > 0 {
			logger.Info("Temperature Set", "temp", temperature, "for", hold)
		} else {
			logger.Info("Temperature Set", "temp", temperature)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tempCmd)
	addSideFlag(tempCmd.Flags())
	tempCmd.Flags().Duration("for", 0, "Hold the temperature for this long (e.g. 90m, 2h)")
	tempCmd.Flags().String("stage", "", "Save as an Autopilot level: bedtime, initial or final")
	tempCmd.MarkFlagsMutuallyExclusive("for", "stage")
}
