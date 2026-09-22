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

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Eight Sleep status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		pods := cli.Status()
		if viper.GetBool("json") {
			return printJSON(cmd.OutOrStdout(), pods, false)
		}
		if len(pods) == 0 {
			return fmt.Errorf("no Eight Sleep devices found on this account")
		}
		for _, pod := range pods {
			if err := renderPodStatus(cmd.OutOrStdout(), pod, cli.UserID()); err != nil {
				return err
			}
		}
		return nil
	},
}

func renderPodStatus(w io.Writer, pod eightsleep.PodStatus, myUserID string) error {
	connection := "OFFLINE"
	if pod.Online {
		connection = "online"
	}
	firmware := pod.FirmwareVersion
	if pod.FirmwareUpdating {
		firmware += " (updating)"
	}
	water := "ok"
	if !pod.HasWater {
		water = "LOW - refill the hub"
	}
	priming := "last primed " + formatTime(pod.LastPrime)
	switch {
	case pod.Priming:
		priming = "in progress"
	case pod.NeedsPriming:
		priming = "NEEDED - run `clim8 prime`"
	}

	model := pod.Model
	if model == "" {
		model = "Pod"
	}
	lines := []string{
		fmt.Sprintf("%s (%s)", model, pod.DeviceID),
		fmt.Sprintf("  Hub:      %s, last heard %s, wifi %d dBm",
			connection, formatTime(pod.LastHeard), pod.WifiSignal),
		"  Firmware: " + firmware,
		"  Water:    " + water,
		"  Priming:  " + priming,
	}
	for _, side := range pod.Sides {
		lines = append(lines,
			fmt.Sprintf("  %-9s %s", sideLabel(side, myUserID), sideSummary(side, pod.Unit)))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func sideLabel(side eightsleep.SideStatus, myUserID string) string {
	label := string(side.Side) + ":"
	if side.UserID != "" && side.UserID == myUserID {
		label = string(side.Side) + "*:"
	}
	return label
}

func sideSummary(side eightsleep.SideStatus, unit eightsleep.UnitOfTemperature) string {
	switch {
	case side.UserID == "":
		return "unassigned"
	case side.Away:
		return "AWAY"
	case !side.Active:
		return "OFF"
	}
	degrees := "°F"
	if unit == eightsleep.Celsius {
		degrees = "°C"
	}
	summary := fmt.Sprintf("ON  %d%s (level %d)", side.Temperature, degrees, side.Level)
	if side.Temperature != side.Target {
		summary += fmt.Sprintf(" -> %d%s (level %d)", side.Target, degrees, side.TargetLevel)
	}
	if side.Activity != "" {
		summary += "  [" + side.Activity + "]"
	}
	return summary
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
