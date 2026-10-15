// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AgentAllowancePatch is a sparse write of the agent allowances: a nil field is
// left as it is. WindowHours is scheduled, never written straight
// (scheduleAgentWindowTx).
type AgentAllowancePatch struct {
	WritesPerWindow *int
	EgressPerWindow *int
	CallsPerWindow  *int
	WindowHours     *int
}

func (p AgentAllowancePatch) writes() ([]pendingWrite, error) {
	out := make([]pendingWrite, 0, 3)
	for _, v := range []struct {
		entry *settings.Entry[int]
		value *int
	}{
		{AgentWritesPerWindow, p.WritesPerWindow},
		{AgentEgressPerWindow, p.EgressPerWindow},
		{AgentCallsPerWindow, p.CallsPerWindow},
	} {
		write, err := encodePatchField(v.entry, v.value)
		if err != nil {
			return nil, err
		}
		out = append(out, write)
	}
	return out, nil
}

// agentAllowanceEntries is the order both readers fill allowanceValues in.
var agentAllowanceEntries = []*settings.Entry[int]{
	AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
	AgentWindowHours, AgentWindowNextHours, AgentWindowNextFrom,
}

// allowanceValues holds the settings in agentAllowanceEntries order.
type allowanceValues [6]int

// policy is the one reading of the six values, for the meter and the page alike.
func (v allowanceValues) policy() agentvolume.Policy {
	plan := agentvolume.WindowPlan{Length: time.Duration(v[3]) * time.Hour}
	if v[4] > 0 {
		plan.Next = time.Duration(v[4]) * time.Hour
		plan.NextFrom = time.Unix(int64(v[5]), 0).UTC()
	}
	return agentvolume.Policy{
		Limits: agentvolume.Limits{Writes: v[0], Egress: v[1], Calls: v[2]},
		Window: plan,
	}
}

// AgentPolicyTx is the machinery read of the allowances, ungated, in one
// statement: the meter applies its answer to every agent call. A surface that
// shows the values to a user reads them through readAgentPolicy instead.
func AgentPolicyTx(ctx context.Context, tx pgx.Tx) (agentvolume.Policy, error) {
	raw, err := settings.ApplyManyTx(ctx, tx,
		AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
		AgentWindowHours, AgentWindowNextHours, AgentWindowNextFrom)
	if err != nil {
		return agentvolume.Policy{}, err
	}
	var values allowanceValues
	for i, entry := range agentAllowanceEntries {
		if err := json.Unmarshal(raw[entry.Key()], &values[i]); err != nil {
			return agentvolume.Policy{}, fmt.Errorf("identity: %s does not hold a whole number: %w", entry.Key(), err)
		}
	}
	return values.policy(), nil
}

// readAgentPolicy reads the allowances for the settings page, each through
// its read gate.
func (s *InstallationSettingsStore) readAgentPolicy(ctx context.Context) (agentvolume.Policy, error) {
	var values allowanceValues
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		for i, entry := range agentAllowanceEntries {
			value, err := settings.GetTx(ctx, tx, entry)
			if err != nil {
				return err
			}
			values[i] = value
		}
		return nil
	})
	if err != nil {
		return agentvolume.Policy{}, fmt.Errorf("identity: reading the agent allowances: %w", err)
	}
	return values.policy(), nil
}

// scheduleAgentWindowTx turns a requested window length into a plan that takes
// over at the next reset. It holds the length setting's write lock, so two
// saves cannot both schedule from the same plan.
func (s *InstallationSettingsStore) scheduleAgentWindowTx(ctx context.Context, tx pgx.Tx, requested *int) error {
	if requested == nil {
		return nil
	}
	raw, err := json.Marshal(*requested)
	if err != nil {
		return fmt.Errorf("identity: encoding %s: %w", AgentWindowHours.Key(), err)
	}
	if err := AgentWindowHours.ValidateJSON(raw); err != nil {
		return err
	}
	if err := settings.LockForWrite(ctx, tx, AgentWindowHours.Key()); err != nil {
		return err
	}
	current, err := AgentPolicyTx(ctx, tx)
	if err != nil {
		return err
	}
	plan := current.Window.Reschedule(s.now(), time.Duration(*requested)*time.Hour)
	nextHours, nextFrom := 0, 0
	if plan.Next > 0 {
		nextHours, nextFrom = int(plan.Next.Hours()), int(plan.NextFrom.Unix())
	}
	for _, w := range []struct {
		entry *settings.Entry[int]
		value int
	}{
		{AgentWindowHours, int(plan.Length.Hours())},
		{AgentWindowNextHours, nextHours},
		{AgentWindowNextFrom, nextFrom},
	} {
		if err := settings.SetTx(ctx, s.settings, tx, w.entry, w.value); err != nil {
			return err
		}
	}
	return nil
}

// PassportConnectionTx answers the connection a passport was minted under, or
// the zero id for a passport a user minted.
func PassportConnectionTx(ctx context.Context, tx pgx.Tx, passport ids.UUID) (ids.UUID, error) {
	var connection *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT oauth_grant_id FROM passport WHERE id = $1`, passport).Scan(&connection); err != nil {
		return ids.Nil, fmt.Errorf("identity: reading passport %s: %w", passport, err)
	}
	if connection == nil {
		return ids.Nil, nil
	}
	return *connection, nil
}
