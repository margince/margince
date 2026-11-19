// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who on our team talks to which buyer seat, read from the activities the
// reader may open at the moment of the read. A withheld exchange never shows a
// date and never becomes a claim that nobody has talked.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedMail writes one exchange between a colleague and a contact, in the given
// audience, as a person wrote it.
func seedPairMail(t *testing.T, e *Env, user, contact ids.UUID, direction, audience string) ids.UUID {
	t.Helper()
	owner := OwnerConn(t)
	activity := ids.NewV7()
	if _, err := owner.Exec(t.Context(), `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, audience)
		VALUES ($1, 'email', 'renewal terms', now() - interval '1 day', $2, 'manual', 'human:x', $3)`,
		activity, direction, audience); err != nil {
		t.Fatal(err)
	}
	LinkActivity(t, owner, activity, "contact", contact)
	userRole, contactRole := "from", "to"
	if direction == "inbound" {
		userRole, contactRole = "to", "from"
	}
	for _, seed := range []struct {
		column string
		id     ids.UUID
		role   string
	}{{"user_id", user, userRole}, {"contact_id", contact, contactRole}} {
		if _, err := owner.Exec(t.Context(), `
			INSERT INTO activity_participant (activity_id, `+seed.column+`, role) VALUES ($1, $2, $3)`,
			activity, seed.id, seed.role); err != nil {
			t.Fatal(err)
		}
	}
	return activity
}

func coverageAs(t *testing.T, e *Env, user ids.UUID, perms principal.Permissions, dealID ids.DealID) crmcontracts.DealCoverage {
	t.Helper()
	ctx := e.As(user, []ids.UUID{e.Team1}, perms)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/deals/"+dealID.String()+"/coverage", nil).WithContext(ctx)
	compose.NewContactGraphReads(e.Pool, e.DB()).GetDealCoverage(rec, req, crmcontracts.Id(dealID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("the coverage page answered %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.DealCoverage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the coverage page: %v", err)
	}
	return out
}

func seatReach(t *testing.T, c crmcontracts.DealCoverage, contact ids.UUID) string {
	t.Helper()
	for _, seat := range c.Stakeholders {
		if ids.UUID(seat.ContactId) == contact {
			if seat.Reach == nil {
				return ""
			}
			return string(*seat.Reach)
		}
	}
	t.Fatalf("no seat for contact %v", contact)
	return ""
}

func linesFor(c crmcontracts.DealCoverage, user, contact ids.UUID) []crmcontracts.DealCoverageLine {
	var out []crmcontracts.DealCoverageLine
	for _, line := range derefLines(c.Lines) {
		if ids.UUID(line.UserId) == user && ids.UUID(line.ContactId) == contact {
			out = append(out, line)
		}
	}
	return out
}

func derefLines(lines *[]crmcontracts.DealCoverageLine) []crmcontracts.DealCoverageLine {
	if lines == nil {
		return nil
	}
	return *lines
}

func TestAWorkspaceMessageDrawsALineWithItsDirectionAndDate(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	seedPairMail(t, e, e.Rep2, buyer, "outbound", "workspace")

	got := coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID)

	lines := linesFor(got, e.Rep2, buyer)
	if len(lines) != 1 || lines[0].LastOutboundAt == nil || lines[0].LastInboundAt != nil || lines[0].LastMetAt != nil {
		t.Fatalf("lines = %+v, want one line where we wrote", lines)
	}
	if time.Since(*lines[0].LastOutboundAt) > 48*time.Hour {
		t.Errorf("the line dates the message %v, want about a day ago", lines[0].LastOutboundAt)
	}
	if lines[0].LastOutboundPrecision == nil || *lines[0].LastOutboundPrecision != "instant" {
		t.Errorf("the line carries precision %v, want instant", lines[0].LastOutboundPrecision)
	}
	if lines[0].ColleagueName == "" {
		t.Error("the line names no colleague")
	}
	if reach := seatReach(t, got, buyer); reach != "reached" {
		t.Errorf("seat reach = %q, want reached", reach)
	}
}

func TestASeatNobodyWroteToIsNotReachedWhenNothingIsWithheld(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)

	got := coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID)

	if got.Lines == nil || len(*got.Lines) != 0 {
		t.Errorf("lines = %v, want present and empty", got.Lines)
	}
	if reach := seatReach(t, got, buyer); reach != "not_reached" {
		t.Errorf("seat reach = %q, want not_reached", reach)
	}
}

func TestAMessageOutsideTheReadersAudienceShowsNoDateAndNoGap(t *testing.T) {
	for _, audience := range []string{"participants", "selected"} {
		t.Run(audience, func(t *testing.T) {
			e := Setup(t)
			buyer := e.SeedContact(t, "Mai Tran", nil)
			dealID := seatedDeal(t, e, buyer)
			seedPairMail(t, e, e.Rep2, buyer, "outbound", audience)

			outside := coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID)
			if lines := linesFor(outside, e.Rep2, buyer); len(lines) != 0 {
				t.Errorf("a reader outside the audience was given the exchange: %+v", lines)
			}
			if reach := seatReach(t, outside, buyer); reach != "unknown" {
				t.Errorf("seat reach = %q, want unknown: a withheld message must not read as no contact", reach)
			}

			inside := coverageAs(t, e, e.Rep2, coverageReaderPerms(true), dealID)
			if lines := linesFor(inside, e.Rep2, buyer); len(lines) != 1 {
				t.Errorf("the participant lost their own line: %+v", lines)
			}
		})
	}
}

func TestANarrowedAudienceHidesTheLineAtOnceWithoutARefold(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	owner := OwnerConn(t)
	activity := seedPairMail(t, e, e.Rep2, buyer, "outbound", "workspace")

	if lines := linesFor(coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID), e.Rep2, buyer); len(lines) != 1 {
		t.Fatalf("the workspace message drew %d lines before narrowing, want 1", len(lines))
	}
	if _, err := owner.Exec(t.Context(), `UPDATE activity SET audience = 'participants' WHERE id = $1`, activity); err != nil {
		t.Fatal(err)
	}

	got := coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID)
	if lines := linesFor(got, e.Rep2, buyer); len(lines) != 0 {
		t.Errorf("a narrowed message still dated a line for an outside reader: %+v", lines)
	}
	if reach := seatReach(t, got, buyer); reach != "unknown" {
		t.Errorf("seat reach = %q, want unknown", reach)
	}
}

func TestAContactTheReaderCannotReadGetsNoLineAndNoState(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	hidden := e.SeedContact(t, "Hidden Buyer", &e.AdminUser)
	makeCapturePrivate(t, e, hidden, e.AdminUser)
	dealID := seatedDeal(t, e, hidden)
	if _, err := owner.Exec(t.Context(), `UPDATE deal SET owner_id = $1 WHERE id = $2`, e.Rep1, dealID.UUID); err != nil {
		t.Fatal(err)
	}
	seedPairMail(t, e, e.Rep2, hidden, "outbound", "workspace")
	perms := coverageReaderPerms(true)
	perms.RowScope = principal.RowScopeOwn

	got := coverageAs(t, e, e.Rep1, perms, dealID)

	for _, seat := range got.Stakeholders {
		if ids.UUID(seat.ContactId) == hidden && seat.Reach != nil {
			t.Errorf("an unreadable seat carried the state %q", *seat.Reach)
		}
	}
	if lines := derefLines(got.Lines); len(lines) != 0 {
		t.Errorf("an unreadable contact appeared in lines: %+v", lines)
	}
}

func TestAColleagueWhoLeftDrawsNoLineAndLeavesTheSeatUnknown(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	seedPairMail(t, e, e.Rep2, buyer, "outbound", "workspace")
	if _, err := owner.Exec(t.Context(), `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep2); err != nil {
		t.Fatal(err)
	}

	got := coverageAs(t, e, e.Rep1, coverageReaderPerms(true), dealID)

	if lines := linesFor(got, e.Rep2, buyer); len(lines) != 0 {
		t.Errorf("a colleague who left drew a line: %+v", lines)
	}
	if reach := seatReach(t, got, buyer); reach != "unknown" {
		t.Errorf("seat reach = %q, want unknown rather than a claim nobody was reached", reach)
	}
}

func TestAReaderWithoutTheActivityGrantGetsNeitherLinesNorStates(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	seedPairMail(t, e, e.Rep2, buyer, "outbound", "workspace")
	perms := coverageReaderPerms(true)
	delete(perms.Objects, "activity")

	got := coverageAs(t, e, e.Rep1, perms, dealID)

	if got.Lines != nil {
		t.Errorf("lines = %v, want absent", *got.Lines)
	}
	if reach := seatReach(t, got, buyer); reach != "" {
		t.Errorf("seat reach = %q, want absent", reach)
	}
}
