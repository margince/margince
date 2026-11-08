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
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// adminRoleKey is the built-in Admin role. A role key is fixed when the role is
// made and never reused. So the preset binds the role, not an editable name.
const adminRoleKey = "admin"

// AdminAllowancePreset is what the built-in Admin role's agents may do where
// nobody chose otherwise. It raises calls and changes for a large reading or
// maintenance task. Sends keep the installation value: such a task is no reason
// to send more mail.
var AdminAllowancePreset = agentvolume.Limits{Calls: 20000, Writes: 2000}

// allowanceCounts pairs each governed count with its role setting, in the
// order allowanceValues holds the installation counts.
var allowanceCounts = []struct {
	counter agentvolume.Counter
	byRole  *config.Setting[map[string]int]
}{
	{agentvolume.Writes, AgentRoleWritesPerWindow},
	{agentvolume.Egress, AgentRoleEgressPerWindow},
	{agentvolume.Calls, AgentRoleCallsPerWindow},
}

// installationChosen says, in allowanceCounts order, whether somebody chose
// each installation count: a stored row or a boot source.
type installationChosen [3]bool

// RoleValues is what each role holds apart from the installation value.
type RoleValues struct {
	// Own is each role's saved values by role key; a zero field uses the
	// installation value.
	Own map[string]agentvolume.Limits
	// Preset is the Admin preset where it binds, zero where it does not.
	Preset agentvolume.Limits
	// Kept is each role's counts saved at 0, kept on the installation value.
	Kept map[string][]agentvolume.Counter
	// PresetIfUnset is the preset where nobody chose the installation value:
	// what Admin takes for a count once it gives its own value up.
	PresetIfUnset agentvolume.Limits
}

// Roles folds the preset into Admin's values, the shape the meter judges by.
func (r RoleValues) Roles() map[string]agentvolume.Limits {
	out := make(map[string]agentvolume.Limits, len(r.Own)+1)
	maps.Copy(out, r.Own)
	for _, count := range allowanceCounts {
		if preset := r.Preset.Of(count.counter); preset > 0 {
			out[adminRoleKey] = out[adminRoleKey].With(count.counter, preset)
		}
	}
	return out
}

// savedRoleMaps is the three role maps as read, by counter.
type savedRoleMaps map[agentvolume.Counter]map[string]int

// roleValuesOf folds the saved role maps into each role's values and applies
// the preset where it binds.
func roleValuesOf(chosen installationChosen, maps savedRoleMaps) RoleValues {
	out := RoleValues{Own: map[string]agentvolume.Limits{}, Kept: map[string][]agentvolume.Counter{}}
	for i, count := range allowanceCounts {
		saved := maps[count.counter]
		for role, value := range saved {
			if value > 0 {
				out.Own[role] = out.Own[role].With(count.counter, value)
			} else {
				out.Kept[role] = append(out.Kept[role], count.counter)
			}
		}
		if !chosen[i] {
			out.PresetIfUnset = out.PresetIfUnset.With(count.counter, AdminAllowancePreset.Of(count.counter))
		}
		if presetBinds(chosen[i], saved) {
			out.Preset = out.Preset.With(count.counter, AdminAllowancePreset.Of(count.counter))
		}
	}
	return out
}

// presetBinds reports whether the Admin preset applies to one count. Nobody
// saved an Admin value for it, and nobody chose the installation value: no row
// is stored and no boot source sets it. An upgrade leaves chosen values alone,
// a saved default included.
func presetBinds(installationChosen bool, saved map[string]int) bool {
	_, adminChosen := saved[adminRoleKey]
	return !adminChosen && !installationChosen
}

// RoleAllowanceSet is one role's changes in a save. Set holds the counts saved
// for the role, where 0 saves the installation value. Unset holds the counts
// whose saved value the role gives up. A count in neither is left as it is.
type RoleAllowanceSet struct {
	RoleKey string
	Set     map[agentvolume.Counter]int
	Unset   []agentvolume.Counter
}

// roleAllowancesField is the patch field a refused role names.
const roleAllowancesField = "agent_role_allowances"

// saveRoleAllowancesTx applies sets to the saved role maps. Only a map a set
// touches is written, so a boot source locking another map refuses nothing. A
// role no set names keeps its values, an archived role's included.
func (s *InstallationSettingsStore) saveRoleAllowancesTx(ctx context.Context, tx pgx.Tx, sets *[]RoleAllowanceSet) error {
	if sets == nil {
		return nil
	}
	roles, err := liveRolesTx(ctx, tx)
	if err != nil {
		return err
	}
	// Locked in one order before the read, so two saves of different roles
	// apply one after the other instead of the later dropping the earlier.
	for _, count := range allowanceCounts {
		if err := settings.LockForWrite(ctx, tx, count.byRole.Key()); err != nil {
			return err
		}
	}
	saved, err := readRoleMapsTx(ctx, tx, s.config)
	if err != nil {
		return err
	}
	// A boot source's map is the configuration's own; edits go to a copy.
	for counter, roleMap := range saved {
		saved[counter] = maps.Clone(roleMap)
	}
	touched := map[agentvolume.Counter]bool{}
	seen := map[string]bool{}
	for _, set := range *sets {
		if err := refuseRoleSet(roles, seen, set); err != nil {
			return err
		}
		for counter, value := range set.Set {
			saved.put(counter, set.RoleKey, value)
			touched[counter] = true
		}
		for _, counter := range set.Unset {
			delete(saved[counter], set.RoleKey)
			touched[counter] = true
		}
	}
	for _, count := range allowanceCounts {
		if !touched[count.counter] {
			continue
		}
		if err := config.SaveTx(ctx, s.config, tx, count.byRole, saved[count.counter]); err != nil {
			return err
		}
	}
	return nil
}

// put saves value for role on counter, starting the map if none was stored.
func (m savedRoleMaps) put(counter agentvolume.Counter, role string, value int) {
	if m[counter] == nil {
		m[counter] = map[string]int{}
	}
	m[counter][role] = value
}

func refuseRoleSet(roles []liveRole, seen map[string]bool, set RoleAllowanceSet) error {
	key := set.RoleKey
	// Giving a value up needs no live role: it is how an archived role's value goes.
	if len(set.Set) > 0 && !slices.ContainsFunc(roles, func(r liveRole) bool { return r.key == key }) {
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
	for _, counter := range set.Unset {
		if _, both := set.Set[counter]; both {
			return &values.ParseError{
				Field: roleAllowancesField, Code: "set_and_unset",
				Message: fmt.Sprintf("%q both sets and unsets its %s; send one", key, counter),
			}
		}
	}
	return nil
}

// RoleAllowance is one live role and the values it holds apart from the
// installation's, for the settings page.
type RoleAllowance struct {
	Key, Name string
	System    bool
	// HoldsPreset marks the built-in Admin role, which the preset is for,
	// whether or not the preset binds any count now.
	HoldsPreset bool
	Own         agentvolume.Limits
	Preset      agentvolume.Limits
	// Kept is the counts saved at 0; PresetIfUnset is what each count takes
	// once the role gives its value up, where that is the preset.
	Kept          []agentvolume.Counter
	PresetIfUnset agentvolume.Limits
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
		allowance := RoleAllowance{Key: r.key, Name: r.name, System: r.system, Own: held.Own[r.key], Kept: held.Kept[r.key]}
		if r.key == adminRoleKey {
			allowance.HoldsPreset, allowance.Preset, allowance.PresetIfUnset = true, held.Preset, held.PresetIfUnset
		}
		out = append(out, allowance)
	}
	return out
}
