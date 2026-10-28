// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a contact case merge writes, and what it must not: nothing for a
// preview or a refusal, everything or nothing for a merge, one winner of two
// racing merges, and the former owner's access on every merge path.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// writtenState is every table a case merge writes, as one comparable string.
func writtenState(t *testing.T, e *integration.Env) string {
	t.Helper()
	return e.WsScalar(t, `SELECT concat_ws(' | ',
		(SELECT count(*) FROM audit_log), (SELECT count(*) FROM event_outbox),
		(SELECT string_agg(record_id::text || subject_id::text || access || coalesce(expires_at::text, ''), ',' ORDER BY id) FROM record_grant),
		(SELECT string_agg(id::text || disposition || left_contact_id::text || right_contact_id::text, ',' ORDER BY id)
		   FROM dedupe_candidate WHERE entity_type = 'contact'),
		(SELECT string_agg(id::text || version || coalesce(owner_id::text, '') || coalesce(archived_at::text, ''), ',' ORDER BY id) FROM contact))`)
}

func TestAPreviewWritesNothingAndAnswersWhatTheMergeDoes(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)
	read := c.read(t, rep, members[0])
	chosen := caseDecision(read, members, members[1], c.e.Rep1)

	before := writtenState(t, c.e)
	preview, err := c.store.PreviewContactCaseMerge(rep, members[0], chosen)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if after := writtenState(t, c.e); after != before {
		t.Fatalf("a preview wrote:\nbefore %s\nafter  %s", before, after)
	}
	if preview.Committed {
		t.Error("a preview says it committed")
	}
	merged, err := c.store.MergeContactCase(rep, members[0], chosen)
	if err != nil {
		t.Fatalf("merging after the preview: %v", err)
	}
	if !mapsEqual(sharesOf(preview), sharesOf(merged)) || len(preview.MergedIds) != len(merged.MergedIds) ||
		preview.Mode != merged.Mode || len(preview.Skipped) != len(merged.Skipped) {
		t.Errorf("the preview answered %+v, the merge did %+v", preview, merged)
	}
}

func TestAStaleRevisionWritesNothing(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1, c.e.Rep1)
	rep := c.rep(c.e.Rep1)
	read := c.read(t, rep, members[0])
	// A colleague's edit moves a member's version, and with it the revision.
	if _, err := c.store.UpdateContact(rep, ids.From[ids.ContactKind](members[2]), contacts.UpdateContactInput{Title: new("Countess")}); err != nil {
		t.Fatalf("editing a member: %v", err)
	}
	before := writtenState(t, c.e)
	_, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, members, members[0], c.e.Rep1))
	if caseCode(err) != "case_changed" {
		t.Fatalf("a merge on a stale revision = %v, want case_changed", err)
	}
	if after := writtenState(t, c.e); after != before {
		t.Errorf("a refused merge wrote:\nbefore %s\nafter  %s", before, after)
	}
}

func TestOfTwoRacingCaseMergesExactlyOneCommits(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1, c.e.Rep1)
	admin := c.e.Admin()
	read := c.read(t, admin, members[0])

	// The barrier: hold every member's row, so both merges are waiting before
	// either can read the case.
	owner := integration.OwnerConn(t)
	barrier, err := owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := barrier.Exec(context.Background(), `SELECT id FROM contact WHERE id = ANY($1) FOR UPDATE`, members); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, survivor := range []ids.UUID{members[0], members[2]} {
		go func() {
			_, err := c.store.MergeContactCase(c.e.Admin(), survivor, caseDecision(read, members, survivor, c.e.Rep1))
			results <- err
		}()
	}
	waitForWaiters(t, owner, 2)
	if err := barrier.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var committed, stale int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			committed++
		case caseCode(err) == "case_changed":
			stale++
		default:
			t.Errorf("a racing merge failed for another reason: %v", err)
		}
	}
	if committed != 1 || stale != 1 {
		t.Errorf("committed %d and refused %d as stale, want exactly one of each", committed, stale)
	}
}

// waitForWaiters returns once n lock requests wait, which is when both merges
// are parked behind the barrier.
func waitForWaiters(t *testing.T, owner *pgx.Conn, n int) {
	t.Helper()
	deadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for deadline.Err() == nil {
		var waiting int
		if err := owner.QueryRow(deadline, `SELECT count(*) FROM pg_locks WHERE NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= n {
			return
		}
	}
	t.Fatalf("fewer than %d merges reached the barrier", n)
}

func TestAMergeThatFailsHalfwayLeavesNothing(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	slices.SortFunc(members, compareUUIDs)
	consentStore := consent.NewStore(c.e.DB())
	// The last member merged holds a stop, and this store has no seam to carry
	// it, so the merge refuses after the first member already merged.
	if err := consentStore.Suppress(c.e.Admin(), consent.SuppressInput{
		ContactID: ids.From[ids.ContactKind](members[2]), Kind: commsauthz.ReasonObjection, Reason: "asked to stop",
	}); err != nil {
		t.Fatalf("recording the stop: %v", err)
	}
	halfWired := contacts.NewStore(c.e.DB()).
		WithSatelliteCarriers(consentStore, introductions.NewStore(c.e.DB(), time.Now)).
		WithGrantCarrier(identity.NewServiceFor(c.e.DB())).
		WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
		WithSettings(configtest.StoredOver(t, c.e.Pool, contacts.ConfigSettings()))
	read := c.read(t, c.e.Admin(), members[0])
	before := writtenState(t, c.e)
	_, err := halfWired.MergeContactCase(c.e.Admin(), members[0], caseDecision(read, members, members[0], c.e.Rep1))
	var unwired *contacts.StopCarrierNotWiredError
	if !errors.As(err, &unwired) {
		t.Fatalf("the half-wired merge = %v, want the refusal to strand a stop", err)
	}
	if after := writtenState(t, c.e); after != before {
		t.Errorf("a failed merge left writes behind:\nbefore %s\nafter  %s", before, after)
	}
}

func compareUUIDs(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) }

func TestEveryMergePathKeepsTheFormerOwnersAccess(t *testing.T) {
	c := setupCases(t)
	admin := c.e.Admin()

	paired := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	pair := c.read(t, admin, paired[0]).Edges[0]
	winner := paired[0]
	if _, err := c.store.DisposeDedupeCandidate(admin, ids.UUID(pair.PairId), "merge", &winner); err != nil {
		t.Fatalf("merging through the pair queue: %v", err)
	}
	if got := grantAccess(t, c.e, paired[0], c.e.Rep2); got != "write" {
		t.Errorf("after a pair merge the former owner holds %q, want write", got)
	}

	direct := c.seedCase(t, c.e.Rep1, c.e.Rep3)
	if _, err := c.store.MergeContact(admin, ids.From[ids.ContactKind](direct[1]), ids.From[ids.ContactKind](direct[0]), nil); err != nil {
		t.Fatalf("merging directly: %v", err)
	}
	if got := grantAccess(t, c.e, direct[0], c.e.Rep3); got != "write" {
		t.Errorf("after a direct merge the former owner holds %q, want write", got)
	}
}

func TestACarriedOwnerKeepsTheTermOfAGrantTheyHold(t *testing.T) {
	c := setupCases(t)
	admin := c.e.Admin()
	grants := identity.NewServiceFor(c.e.DB())
	expires := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	for _, held := range []string{"read", "write"} {
		members := c.seedCase(t, c.e.Rep1, c.e.Rep2)
		if _, err := grants.CreateRecordGrant(admin, identity.CreateGrantInput{
			RecordType: "contact", RecordID: members[0], SubjectType: "user", SubjectID: c.e.Rep2,
			Access: held, ExpiresAt: &expires,
		}); err != nil {
			t.Fatalf("granting %s: %v", held, err)
		}
		before := grantRowState(t, c.e, members[0], c.e.Rep2)
		if _, err := c.store.MergeContact(admin, ids.From[ids.ContactKind](members[1]), ids.From[ids.ContactKind](members[0]), nil); err != nil {
			t.Fatalf("merging: %v", err)
		}
		after := grantRowState(t, c.e, members[0], c.e.Rep2)
		if after.expires != before.expires || after.grantedBy != before.grantedBy {
			t.Errorf("a held %s grant's term moved: %+v → %+v", held, before, after)
		}
		if after.access != "write" {
			t.Errorf("a held %s grant reads %s after the merge, want write", held, after.access)
		}
		if held == "write" && after.version != before.version {
			t.Errorf("a held write grant was rewritten: version %s → %s", before.version, after.version)
		}
	}
}

type grantState struct {
	access, expires, grantedBy string
	version                    string
}

func grantRowState(t *testing.T, e *integration.Env, record, user ids.UUID) grantState {
	t.Helper()
	where := ` FROM record_grant WHERE record_type = 'contact' AND record_id = $1 AND subject_type = 'user' AND subject_id = $2`
	return grantState{
		access:    e.WsScalar(t, `SELECT access`+where, record, user),
		expires:   e.WsScalar(t, `SELECT coalesce(expires_at::text, '')`+where, record, user),
		grantedBy: e.WsScalar(t, `SELECT granted_by::text`+where, record, user),
		version:   e.WsScalar(t, `SELECT version::text`+where, record, user),
	}
}

func TestAMergedSurvivorIsSharedWithItsFormerOwnerOnly(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)
	read := c.read(t, rep, members[0])
	if _, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, members[:2], members[0], c.e.Rep1)); err != nil {
		t.Fatalf("merging: %v", err)
	}
	on := true
	for user, want := range map[ids.UUID]bool{c.e.Rep2: true, c.e.Rep3: false, c.e.Rep1: false} {
		page, _, err := c.store.ListContacts(c.rep(user), contacts.ListContactsInput{SharedWithMe: &on})
		if err != nil {
			t.Fatalf("listing shared contacts: %v", err)
		}
		found := slices.ContainsFunc(page, func(row crmcontracts.Contact) bool { return ids.UUID(row.Id) == members[0] })
		if found != want {
			t.Errorf("shared_with_me for %s lists the survivor: %v, want %v", user, found, want)
		}
	}
	owner := ids.From[ids.UserKind](c.e.Rep1)
	_, _, err := c.store.ListContacts(rep, contacts.ListContactsInput{SharedWithMe: &on, OwnerID: &owner})
	var conflicting *storekit.PredicateError
	if !errors.As(err, &conflicting) || conflicting.Code != storekit.CodeConflictingFilters {
		t.Errorf("shared_with_me with owner_id = %v, want the conflicting-filters refusal", err)
	}
}
