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

	"github.com/spf13/cobra"
)

// primeCmd represents the prime command
var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Start a priming cycle",
	Long: "Start a priming cycle, which circulates water through the pod. " +
		"The Eight Sleep app notifies you when the cycle completes.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := startClient(cmd.Context())
		if err != nil {
			return err
		}
		defer cli.Stop()

		if err := cli.Prime(cmd.Context()); err != nil {
			return err
		}

		// The API acknowledges the request before the pod acts on it, so ask the pod.
		if err := cli.RefreshDevices(cmd.Context()); err != nil {
			return fmt.Errorf("priming was requested, but the pod could not be re-read: %w", err)
		}
		for _, pod := range cli.Status() {
			if pod.Priming {
				logger.Info("Priming started")
				return nil
			}
		}
		logger.Warn("Priming was requested, but the pod does not report it yet; " +
			"check `clim8 status` in a minute")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(primeCmd)
}
