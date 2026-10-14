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
)

// AgentAllowancePatch is a sparse write of the agent allowances: a nil field is
// left as it is. WindowHours is scheduled, never written straight
// (scheduleAgentWindowTx).
type AgentAllowancePatch struct {
	ReadsPerWindow  *int
	WritesPerWindow *int
	EgressPerWindow *int
	CallsPerWindow  *int
	WindowHours     *int
}

func (p AgentAllowancePatch) writes() ([]pendingWrite, error) {
	out := make([]pendingWrite, 0, 4)
	for _, v := range []struct {
		entry *settings.Entry[int]
		value *int
	}{
		{AgentReadsPerWindow, p.ReadsPerWindow},
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

// AgentPolicyTx reads the allowances and window plan in one statement. It is
// the one reader of these settings. The meter applies its answer to every agent
// call, and the settings page shows the same answer.
func AgentPolicyTx(ctx context.Context, tx pgx.Tx) (agentvolume.Policy, error) {
	raw, err := settings.ApplyManyTx(ctx, tx,
		AgentReadsPerWindow, AgentWritesPerWindow, AgentEgressPerWindow, AgentCallsPerWindow,
		AgentWindowHours, AgentWindowNextHours, AgentWindowNextFrom)
	if err != nil {
		return agentvolume.Policy{}, err
	}
	var reads, writes, egress, calls, hours, nextHours, nextFrom int
	for _, f := range []struct {
		into *int
		key  string
	}{
		{&reads, AgentReadsPerWindow.Key()}, {&writes, AgentWritesPerWindow.Key()},
		{&egress, AgentEgressPerWindow.Key()}, {&calls, AgentCallsPerWindow.Key()},
		{&hours, AgentWindowHours.Key()}, {&nextHours, AgentWindowNextHours.Key()},
		{&nextFrom, AgentWindowNextFrom.Key()},
	} {
		if err := json.Unmarshal(raw[f.key], f.into); err != nil {
			return agentvolume.Policy{}, fmt.Errorf("identity: %s does not hold a whole number: %w", f.key, err)
		}
	}
	plan := agentvolume.WindowPlan{Length: time.Duration(hours) * time.Hour}
	if nextHours > 0 {
		plan.Next = time.Duration(nextHours) * time.Hour
		plan.NextFrom = time.Unix(int64(nextFrom), 0).UTC()
	}
	return agentvolume.Policy{
		Limits: agentvolume.Limits{Reads: reads, Writes: writes, Egress: egress, Calls: calls},
		Window: plan,
	}, nil
}

func (s *InstallationSettingsStore) readAgentPolicy(ctx context.Context) (agentvolume.Policy, error) {
	var out agentvolume.Policy
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = AgentPolicyTx(ctx, tx)
		return err
	})
	if err != nil {
		return agentvolume.Policy{}, fmt.Errorf("identity: reading the agent allowances: %w", err)
	}
	return out, nil
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
