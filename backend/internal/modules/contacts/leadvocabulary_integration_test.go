// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The administered lead vocabularies against Postgres. The migration's seed
// agrees with the scorer's fixture, and a lead's source and reason round-trip.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The migration's seed and the code's fixture name the same seven origins with
// the same weights. A seed edited without the fixture, or the reverse, fails
// here. Otherwise a fresh installation would score differently than the unit
// tests say.
func TestLeadOriginSeedMatchesTheScorerFixture(t *testing.T) {
	e := setupPromoteConsent(t)
	var seeded OriginIntents
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) (err error) {
		seeded, err = LoadOriginIntents(e.ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("load intents: %v", err)
	}
	if len(seeded) != len(seededOriginIntents) {
		t.Fatalf("seeded origins = %v, want exactly the fixture %v", seeded, seededOriginIntents)
	}
	for key, want := range seededOriginIntents {
		if got := seeded[key]; got != want {
			t.Errorf("seeded %q = %q, want %q", key, got, want)
		}
	}
	reasons, err := e.store.ListLeadDisqualifyReasons(e.ctx)
	if err != nil {
		t.Fatalf("list reasons: %v", err)
	}
	if len(reasons) != 8 {
		t.Errorf("seeded reasons = %d, want the eight stock reasons", len(reasons))
	}
}

// A lead's source is one of the channels. An internal writer that still hands
// an older word lands on its channel, and the list filters by the channel.
func TestALeadSourceLandsOnItsChannel(t *testing.T) {
	e := setupPromoteConsent(t)
	siteread := "siteread"
	cases := []struct {
		name, source, want string
		system             *string
	}{
		{name: "a website form", source: "web_form", want: "webform"},
		{name: "a website read", source: "siteread", system: &siteread, want: "capture"},
		{name: "a retired list key", source: "crawl", want: "api"},
		{name: "a connector stamp", source: "connector:apollo:a-1", want: "api"},
		{name: "a channel", source: "import", want: "import"},
	}
	for _, c := range cases {
		email := ids.NewV7().String()[24:] + "@example.test"
		in := CreateLeadInput{Email: &email, Source: c.source, SourceSystem: c.system}
		if c.system != nil {
			in.SourceID = new("key-" + email)
		}
		lead, _, err := e.store.CreateLead(e.ctx, in)
		if err != nil {
			t.Fatalf("%s: create: %v", c.name, err)
		}
		if lead.Source != c.want || lead.Score != 0 {
			t.Errorf("%s: source %q scores %d, want %q and 0: how a lead arrived is not scored", c.name, lead.Source, lead.Score, c.want)
		}
	}
	webform := "webform"
	listed, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Source: &webform})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Source != webform {
		t.Errorf("source=webform lists %d leads, want the one the form wrote", len(listed))
	}
	crawl := "crawl"
	if _, err := e.store.UpdateLead(e.ctx, ids.From[ids.LeadKind](ids.UUID(listed[0].Id)), UpdateLeadInput{Source: &crawl}); !isSourceFieldError(err) {
		t.Errorf("rewriting how the lead arrived = %v, want a source field error", err)
	}
}

// Disqualifying records the reason and note on the lead, refuses a reason
// that is not active, and the reason's lead_count and RESTRICT delete follow.
func TestDisqualifyRecordsAnActiveReason(t *testing.T) {
	e := setupPromoteConsent(t)
	reasons, err := e.store.ListLeadDisqualifyReasons(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	badTiming := reasons[1]
	lead := e.seedLead(t, "dq@example.test")

	unknown := ids.NewV7()
	var unknownErr *UnknownDisqualifyReasonError
	if _, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{ReasonID: &unknown}); !errors.As(err, &unknownErr) {
		t.Fatalf("unknown reason err = %v, want UnknownDisqualifyReasonError", err)
	}
	note := "Call back in Q3"
	reasonID := ids.UUID(badTiming.Id)
	closed, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{ReasonID: &reasonID, Note: &note})
	if err != nil {
		t.Fatalf("disqualify: %v", err)
	}
	if closed.Status != crmcontracts.LeadStatusDisqualified || closed.DisqualifyReason == nil || *closed.DisqualifyReason != badTiming.Label ||
		closed.DisqualifyNote == nil || *closed.DisqualifyNote != note || closed.DisqualifyReasonId == nil {
		t.Errorf("closed = %+v, want disqualified with reason %q and the note", closed, badTiming.Label)
	}
	if err := e.store.DeleteLeadDisqualifyReason(e.ctx, reasonID); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("deleting a built-in in-use reason err = %v, want ErrConflict", err)
	}
	custom, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: "Went quiet"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.DeleteLeadDisqualifyReason(e.ctx, ids.UUID(custom.Id)); err != nil {
		t.Errorf("deleting an unused custom reason err = %v, want nil", err)
	}
}

// Every disqualification reason mutation publishes, in the transaction that
// writes the row, because a change to configuration is a fact a subscriber needs.
//
// It asserts the label on the wire and not just a row count. After `deleted`
// the row is gone, so the event is the only thing naming what to drop.
func TestEveryLeadVocabularyMutationPublishesItsChange(t *testing.T) {
	e := setupPromoteConsent(t)

	published := func(eventType string) []string {
		t.Helper()
		rows, err := e.store.db.Pool().Query(e.ctx, `
			SELECT (envelope->'payload'->>'change') || ':' ||
			       coalesce(envelope->'payload'->>'key', envelope->'payload'->>'label')
			FROM event_outbox
			WHERE envelope->>'type' = $1`, eventType)
		if err != nil {
			t.Fatalf("read outbox: %v", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		// Outbox payloads are the contract; UUID order is not publication order.
		slices.Sort(out)
		return out
	}

	reason, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: "Procurement freeze"})
	if err != nil {
		t.Fatalf("create reason: %v", err)
	}
	relabelled := "Budget withdrawn"
	if _, err := e.store.UpdateLeadDisqualifyReason(e.ctx, ids.UUID(reason.Id), UpdateLeadDisqualifyReasonInput{Label: &relabelled}); err != nil {
		t.Fatalf("update reason: %v", err)
	}
	if err := e.store.DeleteLeadDisqualifyReason(e.ctx, ids.UUID(reason.Id)); err != nil {
		t.Fatalf("delete reason: %v", err)
	}
	// The label follows the rename. A reason has no key, so the label is its
	// identity, and a stale one would name something gone.
	wantReasons := []string{"created:Procurement freeze", "updated:Budget withdrawn", "deleted:Budget withdrawn"}
	slices.Sort(wantReasons)
	if got := published("lead_disqualify_reason.changed"); !slices.Equal(got, wantReasons) {
		t.Errorf("lead_disqualify_reason.changed = %v, want %v", got, wantReasons)
	}
}

// The version pin an agent's approval was released against is applied HERE, in
// the transaction that writes, under the row lock the write already takes.
//
// Not decoration: redemption commits its own transaction and the caller then
// opens a fresh one, so a check anywhere earlier proves the row was right when
// the approval was consumed, not when the disqualify lands. A lead edited in
// that window must lose to the compare rather than to timing.
func TestDisqualifyRefusesAVersionThePinNoLongerNames(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "pinned@example.test")

	born, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if born.Version == nil {
		t.Fatal("the seeded lead carries no version, so this case can pin nothing")
	}
	at := *born.Version

	// The pin the write was authorised at still names the row: it lands.
	stale := at - 1
	if _, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{}, OnlyAtVersion(&stale)); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a stale pin err = %v, want ErrVersionSkew — the write ran on a row the approval "+
			"does not describe", err)
	}
	closed, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{}, OnlyAtVersion(&at))
	if err != nil {
		t.Fatalf("the current version was refused: %v", err)
	}
	if closed.Status != crmcontracts.LeadStatusDisqualified {
		t.Errorf("status = %v, want disqualified", closed.Status)
	}
}

// No pin attaches no precondition, so a caller that has one and a caller that
// does not spell the call the same way — which is what lets the unapproved
// static-tier path keep working unchanged.
func TestDisqualifyWithNoPinIsUnconditioned(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "unpinned@example.test")

	closed, err := e.store.DisqualifyLead(e.ctx, lead, DisqualifyLeadInput{}, OnlyAtVersion(nil))
	if err != nil {
		t.Fatalf("a nil pin refused the write: %v", err)
	}
	if closed.Status != crmcontracts.LeadStatusDisqualified {
		t.Errorf("status = %v, want disqualified", closed.Status)
	}
}

// A label names one reason, compared trimmed and case-folded: two spellings of
// one reason would split a report on why leads were dropped.
func TestADisqualifyReasonLabelIsHeldOnce(t *testing.T) {
	e := setupPromoteConsent(t)
	_, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: " no budget "})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second spelling of the stock \"No budget\" answered %v, want a conflict", err)
	}
	want := `a disqualification reason called "no budget" already exists; choose a different name`
	if fault, _ := httperr.Classify(err); fault.Status != http.StatusConflict || fault.Code != "name_taken" || fault.Detail != want {
		t.Errorf("the conflict reads %d %s %q, want 409 name_taken %q", fault.Status, fault.Code, fault.Detail, want)
	}
	custom, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: "Procurement freeze"})
	if err != nil {
		t.Fatalf("create reason: %v", err)
	}
	// Reset keeps the reason table, so the row would outlive this test.
	t.Cleanup(func() {
		if err := e.store.DeleteLeadDisqualifyReason(e.ctx, ids.UUID(custom.Id)); err != nil {
			t.Errorf("removing the custom reason: %v", err)
		}
	})
	if _, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: " procurement FREEZE "}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second spelling of a custom reason answered %v, want a conflict", err)
	}
	taken := "No Budget"
	if _, err := e.store.UpdateLeadDisqualifyReason(e.ctx, ids.UUID(custom.Id), UpdateLeadDisqualifyReasonInput{Label: &taken}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a rename onto a held label answered %v, want a conflict", err)
	}
	sortOrder := 5
	if _, err := e.store.UpdateLeadDisqualifyReason(e.ctx, ids.UUID(custom.Id), UpdateLeadDisqualifyReasonInput{SortOrder: &sortOrder}); err != nil {
		t.Fatalf("a reorder that leaves the label alone: %v", err)
	}
}

// vocabularyList drives one administered list. A keyed list takes a caller's own
// key beside the label.
type vocabularyList struct {
	name   string
	keyed  bool
	create func(key, label string) (ids.UUID, error)
	rename func(id ids.UUID, label string) error
	retire func(id ids.UUID) error
}

func leadVocabularyLists(e *promoteConsentEnv) []vocabularyList {
	return []vocabularyList{{
		name: "lead origin", keyed: true,
		create: func(key, label string) (ids.UUID, error) {
			made, err := e.store.CreateLeadOrigin(e.ctx, CreateLeadOriginInput{Key: key, Label: label})
			return ids.UUID(made.Id), err
		},
		rename: func(id ids.UUID, label string) error {
			_, err := e.store.UpdateLeadOrigin(e.ctx, id, UpdateLeadOriginInput{Label: &label})
			return err
		},
		retire: func(id ids.UUID) error {
			_, err := e.store.UpdateLeadOrigin(e.ctx, id, UpdateLeadOriginInput{Active: new(false)})
			return err
		},
	}, {
		name: "disqualification reason",
		create: func(_, label string) (ids.UUID, error) {
			made, err := e.store.CreateLeadDisqualifyReason(e.ctx, CreateLeadDisqualifyReasonInput{Label: label})
			return ids.UUID(made.Id), err
		},
		rename: func(id ids.UUID, label string) error {
			_, err := e.store.UpdateLeadDisqualifyReason(e.ctx, id, UpdateLeadDisqualifyReasonInput{Label: &label})
			return err
		},
		retire: func(id ids.UUID) error {
			_, err := e.store.UpdateLeadDisqualifyReason(e.ctx, id, UpdateLeadDisqualifyReasonInput{Active: new(false)})
			return err
		},
	}}
}

// requireLabelHeldOnce proves a list holds each label once, compared trimmed and
// ignoring case. A retired entry keeps its label; create and rename both conflict.
func requireLabelHeldOnce(t *testing.T, list vocabularyList) {
	t.Helper()
	suffix := ids.NewV7().String()[24:]
	held, err := list.create("", "Freeze "+suffix)
	if err != nil {
		t.Fatalf("creating the first entry: %v", err)
	}
	if err := list.retire(held); err != nil {
		t.Fatalf("retiring it: %v", err)
	}
	if _, err := list.create("", "  FREEZE "+strings.ToUpper(suffix)+" "); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a second spelling of a retired entry's label answered %v, want a conflict", err)
	}
	if list.keyed {
		if _, err := list.create("other_"+suffix, "freeze "+suffix); !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("the label under a key of its own answered %v, want a conflict", err)
		}
	}
	other, err := list.create("", "Thaw "+suffix)
	if err != nil {
		t.Fatalf("creating a second entry: %v", err)
	}
	err = list.rename(other, " freeze "+strings.ToUpper(suffix))
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a rename onto a held label answered %v, want a conflict", err)
	}
	if fault, _ := httperr.Classify(err); fault.Status != http.StatusConflict || fault.Code != "name_taken" {
		t.Errorf("the conflict reads %d %s %q, want 409 name_taken", fault.Status, fault.Code, fault.Detail)
	}
	if err := list.rename(other, "THAW "+suffix); err != nil {
		t.Errorf("a rename that changes only the case of its own label: %v", err)
	}
}

func requireLongLabelRefused(t *testing.T, list vocabularyList) {
	t.Helper()
	long := strings.Repeat("a", values.TextLabelCharacters+1)
	_, err := list.create("", long)
	requireTooLong(t, err, leadOriginLabelField)
	kept, err := list.create("", "Kept "+ids.NewV7().String()[24:])
	if err != nil {
		t.Fatalf("creating an entry to rename: %v", err)
	}
	requireTooLong(t, list.rename(kept, long), leadOriginLabelField)
}

func requireTooLong(t *testing.T, err error, field string) {
	t.Helper()
	fault, _ := httperr.Classify(err)
	if fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 {
		t.Fatalf("answered %v (status %d), want one 422 field refusal", err, fault.Status)
	}
	got := fault.Fields[0]
	if got.Field != field || got.Code != "too_long" || got.Max != values.TextLabelCharacters {
		t.Errorf("refusal = %+v, want %s too_long with max %d", got, field, values.TextLabelCharacters)
	}
}

func TestALeadListHoldsEachLabelOnce(t *testing.T) {
	e := setupPromoteConsent(t)
	keepOriginCatalogAsSeeded(t, e)
	keepDisqualifyReasonsAsSeeded(t, e)
	for _, list := range leadVocabularyLists(e) {
		t.Run(list.name, func(t *testing.T) {
			requireLabelHeldOnce(t, list)
			requireLongLabelRefused(t, list)
		})
	}
}

// keepDisqualifyReasonsAsSeeded removes the reasons a test added and wakes the
// built-ins it retired. A reset keeps the list, so they would outlive the test.
func keepDisqualifyReasonsAsSeeded(t *testing.T, e *promoteConsentEnv) {
	t.Helper()
	t.Cleanup(func() {
		for _, statement := range []string{
			`DELETE FROM lead_disqualify_reason WHERE NOT system`,
			`UPDATE lead_disqualify_reason SET active = true`,
		} {
			if _, err := e.owner.Exec(context.Background(), statement); err != nil {
				t.Errorf("restoring the disqualification reasons: %v", err)
			}
		}
	})
}

// A lead's name holds at most a label's characters, typed or changed, and the
// refusal names full_name before anything is written.
func TestALeadNamePastTheLimitIsRefusedBeforeTheWrite(t *testing.T) {
	e := setupPromoteConsent(t)
	long := strings.Repeat("n", values.TextLabelCharacters+1)
	_, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{Source: "manual", FullName: &long})
	requireTooLong(t, err, contactNameColumn)
	lead := e.seedLead(t, "named-"+ids.NewV7().String()+"@example.test")
	_, err = e.store.UpdateLead(e.ctx, lead, UpdateLeadInput{FullName: &long})
	requireTooLong(t, err, contactNameColumn)
	fits := strings.Repeat("n", values.TextLabelCharacters)
	if _, err := e.store.UpdateLead(e.ctx, lead, UpdateLeadInput{FullName: &fits}); err != nil {
		t.Errorf("a name at the limit was refused: %v", err)
	}
}
