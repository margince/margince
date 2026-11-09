// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordedUsage answers a fixed usage per subject and records what it was asked.
type recordedUsage struct {
	asked  []agentvolume.Holder
	answer map[string]agentvolume.Usage
	err    error
}

func (r *recordedUsage) Usage(_ context.Context, _ ids.UUID, subjects []agentvolume.Holder) (map[string]agentvolume.Usage, error) {
	r.asked = subjects
	return r.answer, r.err
}

func (r *recordedUsage) AllowanceFor(context.Context, []string) agentvolume.Allowance {
	return agentvolume.Allowance{}
}

func (r *recordedUsage) ResetsAt(context.Context) time.Time { return time.Time{} }

func TestOnlyALivePassportIsAskedForUsage(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	live, revoked, expired := passportRowForTest(), passportRowForTest(), passportRowForTest()
	revoked.RevokedAt = &now
	expired.ExpiresAt = now.Add(-time.Hour)
	connection := passportRowForTest()
	connection.Connection = &PassportConnectionRow{GrantID: ids.NewV7(), ClientID: "c", ClientName: "Claude"}
	reader := &recordedUsage{}
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())

	Handlers{}.WithPassportUsage(reader).liveUsage(ctx, []PassportRow{live, revoked, expired, connection}, []string{"admin"}, now)

	want := []string{live.ID.String(), connection.Connection.GrantID.String()}
	if len(reader.asked) != 2 || reader.asked[0].Subject != want[0] || reader.asked[1].Subject != want[1] {
		t.Fatalf("asked for %v, want the live passport and the connection's grant %v", reader.asked, want)
	}
	if !slices.Equal(reader.asked[0].Roles, []string{"admin"}) {
		t.Errorf("a passport was asked for against roles %v, want its owner's", reader.asked[0].Roles)
	}
}

func TestThePassportListSurvivesAnUnreadableCounter(t *testing.T) {
	reader := &recordedUsage{err: errors.New("redis down")}
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())

	usage := Handlers{}.WithPassportUsage(reader).liveUsage(ctx, []PassportRow{passportRowForTest()}, nil, clocktest.Now(t))

	if usage != nil {
		t.Errorf("an unreadable counter answered %v, want no usage", usage)
	}
}

func TestUsageReachesTheWireWithItsState(t *testing.T) {
	resets := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	wire := toAgentUsage(agentvolume.Usage{
		Writes:   agentvolume.CounterUsage{Used: 200, Limit: 200},
		Calls:    agentvolume.CounterUsage{Used: 10, Limit: 4000, Released: 1},
		ResetsAt: resets,
	})

	if wire.Writes.Used != 200 || wire.Calls.Released != 1 || wire.Calls.Limit != 4000 || !wire.ResetsAt.Equal(resets) {
		t.Errorf("the wire carries %+v", wire)
	}
	if wire.State != "stopped" {
		t.Errorf("a used-up write allowance reads %q, want stopped", wire.State)
	}
}
