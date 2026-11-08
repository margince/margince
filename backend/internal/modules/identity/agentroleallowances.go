// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The agent allowances a role holds apart from the installation value, and the
// Admin preset. The meter judges each call by the caller's roles
// (agentvolume.Policy.For); this file answers what each role holds.

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// adminRoleKey is the built-in Admin role. A role key is fixed when the role is
// made and never reused, so the preset binds the role and not a name an admin
// can edit.
const adminRoleKey = "admin"

// AdminAllowancePreset is what the built-in Admin role's agents may do where
// nobody chose otherwise. It raises calls and changes for a large reading or
// maintenance task, and leaves sends at the installation value: such a task is
// no reason to send more mail.
var AdminAllowancePreset = agentvolume.Limits{Calls: 20000, Writes: 2000}

// allowanceCounts pairs each governed count with its installation setting, its
// role setting and the value it ships with.
var allowanceCounts = []struct {
	counter      agentvolume.Counter
	installation *config.Setting[int]
	byRole       *config.Setting[map[string]int]
	shipped      int
}{
	{agentvolume.Writes, AgentWritesPerWindow, AgentRoleWritesPerWindow, agentvolume.DefaultWrites},
	{agentvolume.Egress, AgentEgressPerWindow, AgentRoleEgressPerWindow, agentvolume.DefaultEgress},
	{agentvolume.Calls, AgentCallsPerWindow, AgentRoleCallsPerWindow, agentvolume.DefaultCalls},
}

// RoleValues is what each role holds apart from the installation value.
type RoleValues struct {
	// Own is each role's saved values by role key; a zero field uses the
	// installation value.
	Own map[string]agentvolume.Limits
	// Preset is the Admin preset where it binds, zero where it does not.
	Preset agentvolume.Limits
}

// Roles folds the preset into Admin's values, the shape the meter judges by.
func (r RoleValues) Roles() map[string]agentvolume.Limits {
	out := make(map[string]agentvolume.Limits, len(r.Own)+1)
	for role, own := range r.Own {
		out[role] = own
	}
	for _, count := range allowanceCounts {
		if preset := r.Preset.Of(count.counter); preset > 0 {
			out[adminRoleKey] = out[adminRoleKey].With(count.counter, preset)
		}
	}
	return out
}

// readRoleValues reads the three role maps through read, the gated or the
// machinery reader, and applies the preset against installation.
func readRoleValues(c *config.Config, installation agentvolume.Limits,
	read func(*config.Setting[map[string]int]) (map[string]int, error),
) (RoleValues, error) {
	out := RoleValues{Own: map[string]agentvolume.Limits{}}
	for _, count := range allowanceCounts {
		saved, err := read(count.byRole)
		if err != nil {
			return RoleValues{}, err
		}
		for role, value := range saved {
			if value > 0 {
				out.Own[role] = out.Own[role].With(count.counter, value)
			}
		}
		if presetBinds(c, count.installation, installation.Of(count.counter), count.shipped, saved) {
			out.Preset = out.Preset.With(count.counter, AdminAllowancePreset.Of(count.counter))
		}
	}
	return out, nil
}

// presetBinds reports whether the Admin preset applies to one count: nobody
// saved an Admin value for it, and the installation value is still the one
// Margince ships and no boot source sets. So an upgrade over values an operator
// chose leaves Admin on them.
func presetBinds(c *config.Config, installation *config.Setting[int], value, shipped int, saved map[string]int) bool {
	if _, chosen := saved[adminRoleKey]; chosen {
		return false
	}
	if _, locked := c.Locked(installation.Key()); locked {
		return false
	}
	return value == shipped
}

// RoleAllowanceSet is one role's values in a save. A zero field uses the
// installation value.
type RoleAllowanceSet struct {
	RoleKey string
	Values  agentvolume.Limits
}

// roleAllowancesField is the patch field a refused role names.
const roleAllowancesField = "agent_role_allowances"

// saveRoleAllowancesTx replaces every role's values with sets. A role left out
// holds none of its own, and Admin left out returns to the preset; Admin sent
// with a zero keeps the installation value instead.
func (s *InstallationSettingsStore) saveRoleAllowancesTx(ctx context.Context, tx pgx.Tx, sets *[]RoleAllowanceSet) error {
	if sets == nil {
		return nil
	}
	roles, err := liveRolesTx(ctx, tx)
	if err != nil {
		return err
	}
	maps := make([]map[string]int, len(allowanceCounts))
	for i := range maps {
		maps[i] = map[string]int{}
	}
	seen := map[string]bool{}
	for _, set := range *sets {
		if err := refuseRoleSet(roles, seen, set.RoleKey); err != nil {
			return err
		}
		for i, count := range allowanceCounts {
			if value := set.Values.Of(count.counter); value > 0 || set.RoleKey == adminRoleKey {
				maps[i][set.RoleKey] = value
			}
		}
	}
	for i, count := range allowanceCounts {
		if err := config.SaveTx(ctx, s.config, tx, count.byRole, maps[i]); err != nil {
			return err
		}
	}
	return nil
}

func refuseRoleSet(roles []liveRole, seen map[string]bool, key string) error {
	if !slices.ContainsFunc(roles, func(r liveRole) bool { return r.key == key }) {
		return &values.ParseError{
			Field: roleAllowancesField, Code: "unknown_role",
			Message: fmt.Sprintf("%q is not a live role, so a value for it would bind nobody", key),
		}
	}
	if seen[key] {
		return &values.ParseError{
			Field: roleAllowancesField, Code: "duplicate_role",
			Message: fmt.Sprintf("%q is given twice; send each role once", key),
		}
	}
	seen[key] = true
	return nil
}

// RoleAllowance is one live role and the values it holds apart from the
// installation's, for the settings page.
type RoleAllowance struct {
	Key, Name string
	System    bool
	Own       agentvolume.Limits
	Preset    agentvolume.Limits
}

// liveRole is a role a user can hold, in the order the role list shows them.
type liveRole struct {
	key, name string
	system    bool
}

// liveRolesTx lists the roles a user can hold: the seeded ones in seed order,
// then the ones an admin made, by name.
func liveRolesTx(ctx context.Context, tx pgx.Tx) ([]liveRole, error) {
	rows, err := tx.Query(ctx, `SELECT key, name, is_system FROM role WHERE archived_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("identity: listing the roles: %w", err)
	}
	roles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (liveRole, error) {
		var r liveRole
		return r, row.Scan(&r.key, &r.name, &r.system)
	})
	if err != nil {
		return nil, fmt.Errorf("identity: listing the roles: %w", err)
	}
	seeded := SystemRoleKeys()
	slices.SortFunc(roles, func(a, b liveRole) int {
		return cmp.Or(cmp.Compare(seedRank(seeded, a.key), seedRank(seeded, b.key)), cmp.Compare(a.name, b.name))
	})
	return roles, nil
}

// seedRank places a seeded role by its seed order and every other role after them.
func seedRank(seeded []string, key string) int {
	if i := slices.Index(seeded, key); i >= 0 {
		return i
	}
	return len(seeded)
}

// roleAllowances lists every live role with what it holds.
func roleAllowances(roles []liveRole, held RoleValues) []RoleAllowance {
	out := make([]RoleAllowance, 0, len(roles))
	for _, r := range roles {
		allowance := RoleAllowance{Key: r.key, Name: r.name, System: r.system, Own: held.Own[r.key]}
		if r.key == adminRoleKey {
			allowance.Preset = held.Preset
		}
		out = append(out, allowance)
	}
	return out
}
