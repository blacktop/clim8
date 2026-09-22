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
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/blacktop/clim8/pkg/eightsleep"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// startClient builds an authenticated client from the configured credentials.
func startClient(ctx context.Context) (*eightsleep.Client, error) {
	if viper.GetBool("verbose") {
		logger.SetLevel(log.DebugLevel)
		log.SetLevel(log.DebugLevel)
	}

	email, password := viper.GetString("email"), viper.GetString("password")
	if email == "" || password == "" {
		return nil, errors.New("email and password are required: pass --email and --password, " +
			"set CLIM8_EMAIL and CLIM8_PASSWORD, or add them to ~/.config/clim8/config.yaml")
	}

	cli, err := eightsleep.NewClient(email, password, "Local")
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	if err := cli.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start client: %w", err)
	}
	return cli, nil
}

func addSideFlag(flags *pflag.FlagSet) {
	flags.String("side", "", "Side to control: left, right or both (default: your own side)")
}

func sideFlag(cmd *cobra.Command) (eightsleep.Side, error) {
	value, err := cmd.Flags().GetString("side")
	if err != nil {
		return "", err
	}
	return eightsleep.ParseSide(value)
}

// printJSON writes v as indented JSON, syntax highlighted when highlight is set.
func printJSON(w io.Writer, v any, highlight bool) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	if !highlight {
		_, err := fmt.Fprintln(w, string(data))
		return err
	}
	if err := quick.Highlight(w, string(data)+"\n", "json", "terminal256", "nord"); err != nil {
		return fmt.Errorf("failed to highlight json: %w", err)
	}
	return nil
}
