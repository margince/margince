// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Deciding a Deal Scout suggestion, and who may see one, against a real
// database. The evidence is written by the product's writers (see
// dealscout_integration_test.go for the kit).

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scoutRepPerms is a rep who reads accounts and may settle signals.
func scoutRepPerms() principal.Permissions {
	perms := integration.AccountRepPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["signal"] = principal.ObjectGrant{Read: true, Update: true}
	return perms
}

func (e *scoutEnv) rep(user ids.UUID, team ids.UUID) context.Context {
	return e.As(user, []ids.UUID{team}, scoutRepPerms())
}

// decider is the deals store with the acceptance effects the server wires.
func (e *scoutEnv) decider() *deals.Store {
	return e.Deals.WithSuggestionEffects(dealSuggestionEffects{})
}

// onlySuggestion answers the company's one suggestion as the reader sees it.
func (e *scoutEnv) onlySuggestion(ctx context.Context, t *testing.T, company ids.UUID) deals.Suggestion {
	t.Helper()
	shown := e.suggestions(ctx, t, company)
	if len(shown) != 1 {
		t.Fatalf("the company shows %d suggestions, want 1", len(shown))
	}
	return shown[0]
}

func TestADismissalHidesTheSuggestionFromEveryoneAndIsAudited(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	rep1 := e.rep(e.Rep1, e.Team1)
	suggestion := e.onlySuggestion(rep1, t, acme)

	if _, err := e.decider().DismissSuggestion(rep1, suggestion.ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	for name, reader := range map[string]context.Context{"the dismisser": rep1, "a colleague": e.rep(e.Rep3, e.Team2), "an admin": e.Admin()} {
		if shown := e.suggestions(reader, t, acme); len(shown) != 0 {
			t.Errorf("%s is still shown %d suggestions after the dismissal", name, len(shown))
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_suggestion' AND entity_id = $1
		AND action = 'update' AND after->>'state' = 'dismissed'`, suggestion.ID); n != 1 {
		t.Fatalf("%d audit rows record the dismissal, want 1", n)
	}
	if by := e.WsScalar(t, `SELECT decided_by::text FROM deal_suggestion WHERE id = $1`, suggestion.ID); by != e.Rep1.String() {
		t.Fatalf("decided_by = %s, want the rep who dismissed it", by)
	}
}

func TestADismissalReArmsOnlyForNewerEvidence(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	suggestion := e.onlySuggestion(e.Admin(), t, acme)
	if _, err := e.decider().DismissSuggestion(e.Admin(), suggestion.ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	dismissedAt, err := time.Parse(time.RFC3339Nano,
		e.WsScalar(t, `SELECT to_char(decided_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM deal_suggestion WHERE id = $1`, suggestion.ID))
	if err != nil {
		t.Fatalf("reading when it was dismissed: %v", err)
	}

	// A meeting captured late, but held before the dismissal, is what the
	// rep already judged.
	e.meeting(e.Admin(), t, "Earlier call", &dana, e.daysAgo(2))
	e.now = dismissedAt.Add(time.Hour)
	e.pass(t)
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("evidence older than the dismissal raised %d suggestions, want none", n)
	}

	e.meeting(e.Admin(), t, "Budget meeting", &dana, dismissedAt.Add(time.Minute))
	e.pass(t)
	fresh := e.onlySuggestion(e.Admin(), t, acme)
	if fresh.ID == suggestion.ID || len(fresh.Evidence) != 1 || fresh.Evidence[0].Title != "Budget meeting" {
		t.Fatalf("after newer evidence the company shows %+v, want a new suggestion citing only the new meeting", fresh)
	}
}

func TestAcceptingOpensTheDealWithTheRepsCorrections(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	rep1 := e.rep(e.Rep1, e.Team1)
	ours := e.meeting(rep1, t, "Scoping workshop", &dana, e.daysAgo(3))
	theirs := e.meeting(e.rep(e.Rep3, e.Team2), t, "Intro call", &dana, e.daysAgo(4))
	mail := e.email(t, "Next steps", "inbound", acme, e.daysAgo(6))
	opportunity := e.signal(t, "new_opportunity", acme, mail, e.daysAgo(5), ids.Nil)
	commitment := e.signal(t, "commitment_made", acme, mail, e.daysAgo(2), ids.Nil)
	e.pass(t)
	suggestion := e.onlySuggestion(rep1, t, acme)

	name, amount, currency := "Acme platform rollout", int64(4_200_000), "EUR"
	closes := e.now.AddDate(0, 2, 0).Truncate(24 * time.Hour)
	out, err := e.decider().AcceptSuggestion(rep1, suggestion.ID, deals.AcceptSuggestionInput{
		Name: &name, AmountMinor: &amount, Currency: &currency, CloseDate: &closes,
	})
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}
	deal, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](out.DealID), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	if deal.Name != name || deal.AmountMinor == nil || *deal.AmountMinor != amount ||
		deal.CompanyId == nil || ids.UUID(*deal.CompanyId) != acme || deal.Source != "agent:dealscout" {
		t.Fatalf("deal = %+v, want the rep's name and amount on Acme, sourced from the scout", deal)
	}
	if deal.CloseDateProvisional != nil && *deal.CloseDateProvisional {
		t.Fatal("a close date the rep chose is marked provisional")
	}
	linked := func(activity ids.UUID) int {
		return e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, activity, out.DealID)
	}
	if linked(ours) != 1 {
		t.Error("the rep's own meeting was not filed under the deal")
	}
	if linked(theirs) != 0 || len(out.Unlinked) != 1 || out.Unlinked[0] != theirs {
		t.Errorf("unlinked = %v, want exactly the other team's meeting, left where it was", out.Unlinked)
	}
	if out.Acknowledged != 2 {
		t.Errorf("acknowledged %d signals, want the 2 that raised it", out.Acknowledged)
	}
	for _, signal := range []ids.UUID{opportunity, commitment} {
		if n := e.WsCount(t, `SELECT count(*) FROM signal_resolution WHERE signal_id = $1 AND source = 'deal_scout'`, signal); n != 1 {
			t.Errorf("signal %s has %d deal_scout resolutions, want 1", signal, n)
		}
	}
	if n := e.stored(t, acme, deals.SuggestionAccepted); n != 1 {
		t.Fatalf("%d accepted suggestions stored, want 1", n)
	}

	_, err = e.decider().AcceptSuggestion(rep1, suggestion.ID, deals.AcceptSuggestionInput{})
	var decided *deals.SuggestionDecidedError
	if !errors.As(err, &decided) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a second accept = %v, want a conflict naming the decided suggestion", err)
	}
}

func TestAcceptingWithoutADateLeavesTheSuggestionsOwn(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	suggestion := e.onlySuggestion(e.Admin(), t, acme)

	out, err := e.decider().AcceptSuggestion(e.Admin(), suggestion.ID, deals.AcceptSuggestionInput{})
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}
	deal, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](out.DealID), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	if deal.Name != "Acme GmbH" || deal.StageId == nil || ids.UUID(*deal.StageId) != e.open.UUID || deal.AmountMinor != nil {
		t.Fatalf("deal = %+v, want the suggestion's name in the first open stage with no amount", deal)
	}
	if deal.ExpectedCloseDate != nil || (deal.CloseDateProvisional != nil && *deal.CloseDateProvisional) {
		t.Fatalf("a suggestion with no date opened a deal dated %v provisional=%v", deal.ExpectedCloseDate, deal.CloseDateProvisional)
	}
}

func TestASuggestionIsShownOnlyToAReaderWhoMaySeeAllOfItsEvidence(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	rep1, rep3 := e.rep(e.Rep1, e.Team1), e.rep(e.Rep3, e.Team2)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(5))
	private := e.meeting(rep1, t, "Pricing call", &dana, e.daysAgo(3))
	e.pass(t)
	if shown := e.suggestions(rep3, t, acme); len(shown) != 1 {
		t.Fatalf("before the limit a colleague is shown %d suggestions, want 1", len(shown))
	}

	if _, err := e.Activities.SetAudience(rep1, ids.From[ids.ActivityKind](private),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the meeting: %v", err)
	}
	if shown := e.suggestions(rep1, t, acme); len(shown) != 1 {
		t.Fatalf("the meeting's own participant is shown %d suggestions, want 1", len(shown))
	}
	for name, reader := range map[string]context.Context{"a colleague": rep3, "an admin": e.Admin()} {
		if shown := e.suggestions(reader, t, acme); len(shown) != 0 {
			t.Errorf("%s who may not read one meeting is shown %d suggestions", name, len(shown))
		}
		if n, err := e.Deals.CountOpenSuggestions(reader); err != nil || n != 0 {
			t.Errorf("%s counts %d open suggestions (err %v), want 0", name, n, err)
		}
		if _, err := e.decider().DismissSuggestion(reader, e.onlySuggestion(rep1, t, acme).ID); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s dismissing a suggestion they cannot see = %v, want not found", name, err)
		}
	}
}

func TestASuggestionWhoseEvidenceIsArchivedIsHiddenAndThenRetired(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	meeting := e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	e.onlySuggestion(e.Admin(), t, acme)

	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](meeting), nil); err != nil {
		t.Fatalf("archiving the meeting: %v", err)
	}
	if shown := e.suggestions(e.Admin(), t, acme); len(shown) != 0 {
		t.Fatalf("a suggestion over an archived meeting is still shown %d times", len(shown))
	}
	if pass := e.pass(t); pass.Superseded != 1 {
		t.Fatalf("the next pass superseded %d suggestions, want 1", pass.Superseded)
	}
	if n := e.stored(t, acme, deals.SuggestionSuperseded); n != 1 {
		t.Fatalf("%d superseded suggestions stored, want 1", n)
	}
}

func TestAnOpenDealSupersedesTheSuggestion(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	company := ids.From[ids.CompanyKind](acme)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Opened by hand", PipelineID: e.pipeline, StageID: e.open, CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatalf("opening the deal: %v", err)
	}
	if pass := e.pass(t); pass.Superseded != 1 {
		t.Fatalf("the pass after a deal opened superseded %d suggestions, want 1", pass.Superseded)
	}
	if shown := e.suggestions(e.Admin(), t, acme); len(shown) != 0 {
		t.Fatalf("a company with an open deal still shows %d suggestions", len(shown))
	}
}

func TestASuggestionFromSignalsIsHiddenWhenAMessageTheyCiteIsLimited(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	rep1, rep3 := e.rep(e.Rep1, e.Team1), e.rep(e.Rep3, e.Team2)
	subject, inbound, at := "Next steps", "inbound", e.daysAgo(6)
	mail, _, err := e.Activities.LogActivity(rep1, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: acme}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	cited := ids.UUID(mail.Id)
	e.signal(t, "new_opportunity", acme, cited, e.daysAgo(5), ids.Nil)
	e.signal(t, "commitment_made", acme, e.email(t, "Confirmed", "inbound", acme, at), e.daysAgo(2), ids.Nil)
	e.pass(t)
	if shown := e.suggestions(rep3, t, acme); len(shown) != 1 {
		t.Fatalf("before the limit a colleague is shown %d suggestions, want 1", len(shown))
	}

	if _, err := e.Activities.SetAudience(rep1, ids.From[ids.ActivityKind](cited),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail: %v", err)
	}
	if shown := e.suggestions(rep1, t, acme); len(shown) != 1 {
		t.Fatalf("the mail's own author is shown %d suggestions, want 1", len(shown))
	}
	if shown := e.suggestions(rep3, t, acme); len(shown) != 0 {
		t.Fatalf("a colleague who may not read the cited mail is shown %d suggestions", len(shown))
	}
}
