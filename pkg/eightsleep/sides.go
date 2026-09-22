package eightsleep

import (
	"errors"
	"fmt"
	"strings"
)

// Side selects which side(s) of the pod a command applies to.
type Side string

const (
	// SideMine targets the authenticated user's own side.
	SideMine  Side = ""
	SideLeft  Side = "left"
	SideRight Side = "right"
	SideBoth  Side = "both"
)

// ParseSide parses a --side value. An empty string means the authenticated user's side.
func ParseSide(s string) (Side, error) {
	switch side := Side(strings.ToLower(strings.TrimSpace(s))); side {
	case SideMine, SideLeft, SideRight, SideBoth:
		return side, nil
	default:
		return "", fmt.Errorf("invalid side %q (must be left, right or both)", s)
	}
}

// sideIsAway reports whether the user assigned to a side is away. awaySides keeps the true
// assignment at all times; while a user is away their top-level slot is blanked or taken over
// by the user who is still present.
func sideIsAway(current, assigned string) bool {
	return assigned != "" && current != assigned
}

// sideUserIDs returns the users assigned to each side of the device, preferring awaySides
// because away mode blanks or collapses the top-level IDs.
func sideUserIDs(d Device) (left, right string) {
	left, right = d.LeftUserID, d.RightUserID
	awayLeft, awayRight := d.AwaySides.LeftUserID, d.AwaySides.RightUserID
	if awayLeft != "" && awayRight != "" && awayLeft != awayRight {
		return awayLeft, awayRight
	}
	if left == "" {
		left = awayLeft
	}
	if right == "" {
		right = awayRight
	}
	return left, right
}

// userIDs resolves a side selection to the user IDs the API calls must target.
func (c *Client) userIDs(side Side) ([]string, error) {
	if side == SideMine {
		return []string{c.me.ID}, nil
	}
	device, err := c.primaryDevice()
	if err != nil {
		return nil, err
	}
	left, right := sideUserIDs(device)

	var ids []string
	switch side {
	case SideLeft:
		ids = []string{left}
	case SideRight:
		ids = []string{right}
	case SideBoth:
		ids = []string{left, right}
		if left == right {
			ids = []string{left}
		}
	case SideMine:
	}

	var assigned []string
	for _, id := range ids {
		if id != "" {
			assigned = append(assigned, id)
		}
	}
	if len(assigned) == 0 {
		return nil, fmt.Errorf("no user is assigned to side %q of device %s", side, device.ID)
	}
	if side == SideBoth && len(assigned) == 1 && left != right {
		return nil, errors.New("only one side of the pod has a user assigned; use that side instead")
	}
	return assigned, nil
}

// userID resolves a side selection that must name exactly one user.
func (c *Client) userID(side Side) (string, error) {
	if side == SideBoth {
		return "", errors.New("this command applies to one user; use --side left or --side right")
	}
	ids, err := c.userIDs(side)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}
