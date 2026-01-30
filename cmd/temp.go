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

	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// tempCmd represents the temp command
var tempCmd = &cobra.Command{
	Use:     "temp <temperature>",
	Short:   "Set the temperature of Eight Sleep Pod",
	Long:    "Set the temperature of Eight Sleep Pod. Temperature must include unit (F for Fahrenheit or C for Celsius).",
	Example: "  clim8 temp 68F\n  clim8 temp 24C\n  clim8 temp 72F",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if viper.GetBool("verbose") {
			logger.SetLevel(log.DebugLevel)
		}

		temperature := args[0]

		cli, err := eightsleep.NewClient(
			viper.GetString("email"),
			viper.GetString("password"),
			"America/New_York",
		)
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		defer cli.Stop()

		if err := cli.Start(cmd.Context()); err != nil {
			return fmt.Errorf("failed to start client: %w", err)
		}

		if err := cli.TurnOn(cmd.Context()); err != nil {
			return err
		}

		if err := cli.SetTemperature(cmd.Context(), temperature); err != nil {
			// Attempt rollback - best effort, don't fail if rollback fails
			logger.Warn("SetTemperature failed, attempting rollback by turning off", "err", err)
			if rollbackErr := cli.TurnOff(cmd.Context()); rollbackErr != nil {
				logger.Error("rollback (TurnOff) also failed", "err", rollbackErr)
			}
			return err
		}
		logger.Info("Temperature Set", "temp", temperature)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tempCmd)
}
