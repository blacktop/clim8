package eightsleep

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// SetAway starts or ends away mode for the selected side(s).
//
// The backend applies away mode to the account's current device, so on an account with
// several pods it can land on a different pod than the one a side was resolved from.
func (c *Client) SetAway(ctx context.Context, side Side, away bool) error {
	userIDs, err := c.userIDs(side)
	if err != nil {
		return err
	}
	action := "end"
	if away {
		action = "start"
	}
	// A timestamp in the past makes the change take effect immediately.
	when := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	body := map[string]any{"awayPeriod": map[string]string{action: when}}
	for _, userID := range userIDs {
		url := c.appAPIURL + "/v1/users/" + userID + "/away-mode"
		if err := c.doJSON(ctx, http.MethodPut, url, body, nil); err != nil {
			return fmt.Errorf("failed to %s away mode for user %s: %w", action, userID, err)
		}
	}
	return nil
}
