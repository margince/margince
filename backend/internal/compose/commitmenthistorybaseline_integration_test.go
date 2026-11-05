// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// baselineAsOf is two days after baselineMail was sent, so normal policy writes the task.
var baselineAsOf = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// baselineMail is sent at 00:30 on 19 October in Berlin, still 18 October in New York.
const baselineMail = "Sun, 18 Oct 2026 22:30:00 +0000"

const beforeBaseline = CommitmentOutcome(activities.CommitmentBeforeBaseline)

// connectProvider adds a second mailbox for a seat under the address seatAddress gave it.
func connectProvider(t *testing.T, e *integration.Env, seat ids.UUID, provider string) {
	t.Helper()
	address := seatAddress(t, e, seat)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
			VALUES ($1, $2, 'connected', 'vault:test', $3)
			ON CONFLICT (user_id, provider) DO NOTHING`, seat, provider, address)
		return err
	}); err != nil {
		t.Fatalf("connecting %s: %v", provider, err)
	}
}

func captureMailVia(t *testing.T, e *integration.Env, seat ids.UUID, provider string, raw []byte) {
	t.Helper()
	captureMailAs(t, e, seat, provider, seatAddress(t, e, seat), raw)
}

// captureMailAs captures for a mailbox whose address is already on its connection.
func captureMailAs(t *testing.T, e *integration.Env, seat ids.UUID, provider, self string, raw []byte) {
	t.Helper()
	parsed, err := mailmap.Parse(raw, self)
	if err != nil {
		t.Fatalf("parsing the message: %v", err)
	}
	ctx := threadConnectorCtx(e, seat)
	actor, _ := principal.Actor(ctx)
	actor.ID = "connector:" + provider
	if _, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(principal.WithActor(ctx, actor), parsed.ToRecord(provider, raw)); err != nil {
		t.Fatalf("capturing the message through %s: %v", provider, err)
	}
}

// setBaseline goes through the registry's own writer, as the settings screen does.
func setBaseline(t *testing.T, e *integration.Env, seat ids.UUID, provider, day, zone string) {
	t.Helper()
	registry := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), backfillAuthority{}, keyvault.NewMemory())
	registry.Register(plainSyncConnector{name: provider})
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
	})
	var date *time.Time
	if day != "" {
		parsed, err := time.Parse(time.DateOnly, day)
		if err != nil {
			t.Fatal(err)
		}
		date = &parsed
	}
	if _, err := registry.SetHistoryBaseline(ctx, provider, date, zone); err != nil {
		t.Fatalf("setting the %s baseline: %v", provider, err)
	}
}

func mailSource(t *testing.T, e *integration.Env, messageID string) ids.UUID {
	t.Helper()
	return ids.MustParse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, messageID))
}

func dispatchFor(t *testing.T, e *integration.Env, source, seat ids.UUID) CommitmentOutcome {
	t.Helper()
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+seat.String(), ids.NewV7())
	c := Commitment{
		Meaning: openCommitmentMeaning("", ""), AsOf: baselineAsOf, SourceActivityID: source, ReadMessages: []ids.UUID{source},
		Extractor: signalScanActor, Locator: commitmentLocator(source, claimKindOurs, seat.String(), "Send pricing"),
		Seat: &seat, Summary: "Send pricing", Confidence: 0.95,
	}
	var outcome CommitmentOutcome
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		outcome, _, err = NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB())).DispatchTx(ctx, tx, c, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestAMailboxBaselineWithholdsSuggestionsStrictlyBeforeItsLocalDate(t *testing.T) {
	cases := []struct {
		name, day, zone string
		want            CommitmentOutcome
	}{
		{"no baseline", "", "", CommitmentTaskWritten},
		{"the baseline is the local day the mail was sent", "2026-10-19", "Europe/Berlin", CommitmentTaskWritten},
		{"the baseline is the local day after", "2026-10-20", "Europe/Berlin", beforeBaseline},
		{"the baseline is read in its own zone", "2026-10-19", "America/New_York", beforeBaseline},
		{"the baseline is the local day before", "2026-10-18", "Europe/Berlin", CommitmentTaskWritten},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			captureMailVia(t, e, e.Rep1, "gmail", threadMail(seatAddress(t, e, e.Rep1), "boundary@buyer.test", baselineMail, ""))
			if tc.day != "" {
				setBaseline(t, e, e.Rep1, "gmail", tc.day, tc.zone)
			}
			source := mailSource(t, e, "boundary@buyer.test")
			if got := dispatchFor(t, e, source, e.Rep1); got != tc.want {
				t.Fatalf("outcome = %q, want %q", got, tc.want)
			}
			wantTasks := map[bool]int{true: 1, false: 0}[tc.want == CommitmentTaskWritten]
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != wantTasks {
				t.Errorf("tasks = %d, want %d", got, wantTasks)
			}
			if tc.want == beforeBaseline {
				if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE outcome = $1 AND NOT current_work`, string(beforeBaseline)); got != 1 {
					t.Errorf("withheld decisions = %d, want one reviewable decision", got)
				}
				if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, source); got != 1 {
					t.Error("the withheld mail is no longer on the timeline")
				}
			}
		})
	}
}

func TestAMailboxBaselineLeavesExistingWorkAndRepeatsTheSameAnswer(t *testing.T) {
	e := integration.Setup(t)
	raw := threadMail(seatAddress(t, e, e.Rep1), "existing@buyer.test", baselineMail, "")
	captureMailVia(t, e, e.Rep1, "gmail", raw)
	source := mailSource(t, e, "existing@buyer.test")
	if got := dispatchFor(t, e, source, e.Rep1); got != CommitmentTaskWritten {
		t.Fatalf("outcome without a baseline = %q, want a task", got)
	}
	task := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'task'`))
	for _, done := range []bool{true, false} {
		if _, err := e.Activities.UpdateActivity(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ActivityKind](task), activities.UpdateActivityInput{IsDone: &done}); err != nil {
			t.Fatal(err)
		}
	}
	setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")
	captureMailVia(t, e, e.Rep1, "gmail", raw)
	for range 2 {
		if got := dispatchFor(t, e, source, e.Rep1); got != CommitmentRemembered {
			t.Fatalf("outcome after setting the baseline = %q, want the existing task remembered", got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND NOT is_done AND archived_at IS NULL`, task); got != 1 {
		t.Error("setting a baseline completed or archived the human-reopened task")
	}
	setBaseline(t, e, e.Rep1, "gmail", "", "")
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND archived_at IS NULL`); got != 1 {
		t.Errorf("tasks after clearing = %d, want the one task", got)
	}
}

func TestAMailboxBaselineAppliesOnlyWhenEveryDeliveringMailboxOfTheSeatExcludes(t *testing.T) {
	for _, both := range []bool{false, true} {
		t.Run(map[bool]string{false: "one of two mailboxes", true: "both mailboxes"}[both], func(t *testing.T) {
			e := integration.Setup(t)
			connectProvider(t, e, e.Rep1, "graph")
			raw := threadMail(seatAddress(t, e, e.Rep1), "two-mailboxes@buyer.test", baselineMail, "")
			captureMailVia(t, e, e.Rep1, "gmail", raw)
			captureMailVia(t, e, e.Rep1, "graph", raw)
			if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE user_id = $1 AND delivered_by = ARRAY['gmail:0','graph:0']`, e.Rep1); got != 1 {
				t.Fatalf("import rows naming both mailboxes = %d, want 1", got)
			}
			setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")
			want := CommitmentTaskWritten
			if both {
				setBaseline(t, e, e.Rep1, "graph", "2026-10-20", "Europe/Berlin")
				want = beforeBaseline
			}
			if got := dispatchFor(t, e, mailSource(t, e, "two-mailboxes@buyer.test"), e.Rep1); got != want {
				t.Errorf("outcome = %q, want %q", got, want)
			}
		})
	}
}

func TestAMailboxBaselineNeverWithholdsAnotherSeatsWork(t *testing.T) {
	e := integration.Setup(t)
	to := seatAddress(t, e, e.Rep1) + ", " + seatAddress(t, e, e.Rep2)
	raw := threadMail(to, "shared@buyer.test", baselineMail, "")
	captureMailVia(t, e, e.Rep1, "gmail", raw)
	captureMailVia(t, e, e.Rep2, "gmail", raw)
	setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")
	source := mailSource(t, e, "shared@buyer.test")
	if got := dispatchFor(t, e, source, e.Rep2); got != CommitmentTaskWritten {
		t.Errorf("the colleague's outcome = %q, want their task", got)
	}
	if got := dispatchFor(t, e, source, e.Rep1); got != beforeBaseline {
		t.Errorf("the baseline owner's outcome = %q, want %q", got, beforeBaseline)
	}
}

func TestAMailboxBaselineNeedsProofOfTheDeliveringMailbox(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "no provenance", true: "provenance in captured_by only"}[legacy], func(t *testing.T) {
			e := integration.Setup(t)
			captureMailVia(t, e, e.Rep1, "gmail", threadMail(seatAddress(t, e, e.Rep1), "provenance@buyer.test", baselineMail, ""))
			setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")
			source := mailSource(t, e, "provenance@buyer.test")
			e.WsExec(t, `UPDATE capture_import SET delivered_by = '{}' WHERE activity_id = $1`, source)
			want := beforeBaseline
			if !legacy {
				e.WsExec(t, `UPDATE activity SET captured_by = 'connector:gmail' WHERE id = $1`, source)
				want = CommitmentTaskWritten
			}
			if got := dispatchFor(t, e, source, e.Rep1); got != want {
				t.Errorf("outcome = %q, want %q", got, want)
			}
		})
	}
}

func TestAMailboxBaselineDoesNotGovernAMeeting(t *testing.T) {
	e := integration.Setup(t)
	seatAddress(t, e, e.Rep1)
	setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")
	at, body, held := baselineAsOf.AddDate(0, 0, -2), "I will send the pricing.", "held"
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Body: &body, OccurredAt: &at, ThreadKey: "meeting-baseline", Source: "manual", MeetingStatus: &held,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := dispatchFor(t, e, ids.UUID(meeting.Id), e.Rep1); got != CommitmentTaskWritten {
		t.Errorf("meeting outcome = %q, want its task", got)
	}
}

// labelledConnector names its account from the credential, so a second grant
// with other bytes is a rebind to another account.
type labelledConnector struct{ plainSyncConnector }

func (labelledConnector) AccountLabel(auth connector.Auth) (string, error) { return string(auth), nil }

func TestAMailboxBaselineSetAfterARebindLeavesThePreviousAccountsMail(t *testing.T) {
	e := integration.Setup(t)
	oldAddress := seatAddress(t, e, e.Rep1)
	captureMailVia(t, e, e.Rep1, "gmail", threadMail(oldAddress, "old-account@buyer.test", baselineMail, ""))

	registry := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), backfillAuthority{}, keyvault.NewMemory())
	registry.Register(labelledConnector{plainSyncConnector{name: "gmail"}})
	human := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
	newAddress := "rebound-" + e.Rep1.String()[:8] + "@ws.example"
	if _, err := registry.Connect(human, "gmail", connector.Auth(newAddress)); err != nil {
		t.Fatalf("rebinding the mailbox: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_connection WHERE user_id = $1 AND provider = 'gmail' AND generation = 1`, e.Rep1); got != 1 {
		t.Fatalf("connections on the new account's generation = %d, want 1", got)
	}
	captureMailAs(t, e, e.Rep1, "gmail", newAddress, threadMail(newAddress, "new-account@buyer.test", baselineMail, ""))
	setBaseline(t, e, e.Rep1, "gmail", "2026-10-20", "Europe/Berlin")

	if got := dispatchFor(t, e, mailSource(t, e, "old-account@buyer.test"), e.Rep1); got != CommitmentTaskWritten {
		t.Errorf("the previous account's mail = %q, want its task", got)
	}
	if got := dispatchFor(t, e, mailSource(t, e, "new-account@buyer.test"), e.Rep1); got != beforeBaseline {
		t.Errorf("the new account's mail = %q, want %q", got, beforeBaseline)
	}
}

func TestAMailboxBaselineReadsTheDateOnTheSeatsOwnCopy(t *testing.T) {
	e := integration.Setup(t)
	to := seatAddress(t, e, e.Rep1) + ", " + seatAddress(t, e, e.Rep2)
	captureMailVia(t, e, e.Rep1, "gmail", threadMail(to, "two-dates@buyer.test", "Sat, 10 Oct 2026 09:00:00 +0000", ""))
	captureMailVia(t, e, e.Rep2, "gmail", threadMail(to, "two-dates@buyer.test", baselineMail, ""))
	source := mailSource(t, e, "two-dates@buyer.test")
	if got := e.WsCount(t, `SELECT count(DISTINCT source_occurred_at) FROM capture_import WHERE activity_id = $1`, source); got != 2 {
		t.Fatalf("distinct source dates = %d, want each seat's own", got)
	}
	setBaseline(t, e, e.Rep2, "gmail", "2026-10-15", "Europe/Berlin")
	if got := dispatchFor(t, e, source, e.Rep2); got != CommitmentTaskWritten {
		t.Errorf("outcome = %q, want the task: this seat's copy is dated after its baseline", got)
	}
}
