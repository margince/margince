// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Who may read the three questions ABOUT the queue.
//
// The board refused below `team` in its own body; the guardrail and the metrics
// checked nothing at all, so the product's answer depended on which endpoint you
// asked. The client hid all three behind the same tier, which is what made the
// gap invisible from a browser — an unmodified client enforced a rule nothing
// else held.
//
// Table-driven over all three, so a fourth reading added later either joins the
// table or is visibly absent from it.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadReadings names each reading and how to ask it, so refusal and admission
// are asserted against the same list.
func leadReadings() []struct {
	name string
	ask  func(*Service, context.Context) error
} {
	return []struct {
		name string
		ask  func(*Service, context.Context) error
	}{
		{"the team board", func(s *Service, ctx context.Context) error {
			_, err := s.TeamBoard(ctx)
			return err
		}},
		{"the hidden-backlog guardrail", func(s *Service, ctx context.Context) error {
			_, err := s.HiddenBacklog(ctx)
			return err
		}},
		{"the response metrics", func(s *Service, ctx context.Context) error {
			_, err := s.ResponseMetrics(ctx, 0)
			return err
		}},
	}
}

// A service whose seams are all unbound.
//
// Deliberate, and it is what makes the refusal test sharp: an unbound seam is
// each reading's SUCCESS path — the guardrail answers a clear backlog and the
// metrics answer an empty window rather than an error. So a refusal here can
// only have come from the tier check, and a missing check reads as a 200
// carrying reassuring zeros rather than as a crash.
func tierService() *Service {
	return &Service{
		teammates: roster([]TeamMember{{UserID: theReader, DisplayName: "the reader"}}),
		now:       func() time.Time { return boardInstant },
	}
}

// aLead is a reader admitted to all three readings, for the tests whose subject
// is what a reading SAYS rather than who may ask it.
//
// Those tests passed a bare context before these gates existed, which was a
// reader with no principal at all — now a refusal. It lives beside the rule it
// satisfies rather than in each file, so a reader of either file finds the tier
// explained where requireLeadTier is.
func aLead() context.Context {
	return boardReaderAt(principal.RowScopeTeam)
}

func TestAnOwnScopedReaderIsRefusedEveryLeadReading(t *testing.T) {
	t.Parallel()

	for _, reading := range leadReadings() {
		t.Run(reading.name, func(t *testing.T) {
			t.Parallel()

			err := reading.ask(tierService(), boardReaderAt(principal.RowScopeOwn))
			if err == nil {
				t.Fatal("a reader who sees only their own work was given a lead's reading — " +
					"an unbound seam answers this with reassuring zeros, so a missing " +
					"tier check looks exactly like a healthy workspace")
			}
		})
	}
}

// The two tiers that reach past the reader are admitted, without which the
// refusals above pass against a gate that refuses everybody.
func TestTeamAndAllScopedReadersGetEveryLeadReading(t *testing.T) {
	t.Parallel()

	for _, reading := range leadReadings() {
		for _, tier := range []principal.RowScope{principal.RowScopeTeam, principal.RowScopeAll} {
			t.Run(reading.name+" at "+string(tier), func(t *testing.T) {
				t.Parallel()

				if err := reading.ask(tierService(), boardReaderAt(tier)); err != nil {
					t.Fatalf("row scope %q was refused: %v", tier, err)
				}
			})
		}
	}
}

// A caller with no actor at all is refused rather than admitted by default.
func TestAReadingWithNoActorIsRefused(t *testing.T) {
	t.Parallel()

	for _, reading := range leadReadings() {
		t.Run(reading.name, func(t *testing.T) {
			t.Parallel()

			if err := reading.ask(tierService(), context.Background()); err == nil {
				t.Fatal("a request carrying no principal was given a lead's reading")
			}
		})
	}
}

// The guardrail refuses BEFORE it answers for an unbound seam.
//
// Order matters here and nowhere else in this file: an unbound waiting seam
// answers `clear: true`, which is the endpoint's healthy answer. Checking the
// tier after that would tell a refused reader their backlog is clear — the one
// statement this surface must never make wrongly, and a lie a 403 does not tell.
func TestTheGuardrailRefusesRatherThanReportingAClearBacklog(t *testing.T) {
	t.Parallel()

	got, err := tierService().HiddenBacklog(boardReaderAt(principal.RowScopeOwn))
	if err == nil {
		t.Fatalf("a refused reader was told the backlog is clear: %+v", got)
	}
	if got.Clear {
		t.Error("the refusal still carried clear=true, which a client may read")
	}
}

// And the admitted reader still gets the unbound seam's honest answer, so the
// refusal above is the tier and not the seam.
func TestAnAdmittedReaderStillReadsAnUnboundGuardrailAsClear(t *testing.T) {
	t.Parallel()

	got, err := tierService().HiddenBacklog(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatalf("a team-scoped reader was refused the guardrail: %v", err)
	}
	if !got.Clear {
		t.Errorf("an installation reading no mail reported held-back work: %+v", got)
	}
}

// The metrics carry the window they measured even with no seam bound, so a
// refused reader is told nothing about what would have been measured.
func TestRefusedMetricsCarryNoWindow(t *testing.T) {
	t.Parallel()

	got, err := tierService().ResponseMetrics(boardReaderAt(principal.RowScopeOwn), 30)
	if err == nil {
		t.Fatalf("a refused reader was given the metrics: %+v", got)
	}
	if !got.From.IsZero() || !got.To.IsZero() {
		t.Errorf("the refusal named the window it would have measured: %+v", got)
	}
}
