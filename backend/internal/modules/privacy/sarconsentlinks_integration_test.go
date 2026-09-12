// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What the Art. 15 export says about the links the workspace mailed a subject.
//
// A grant on file says the subject agreed. It does not say whether they were
// ever asked, when, or whether the link they were sent is one they actually
// answered — and that gap is the difference between a completed double opt-in
// and a row somebody typed. Until this section the export carried the grant and
// not the round trip behind it.
//
// The four outcomes are asserted against rows in each state rather than through
// the minting API: the writer lives in the consent module, which privacy may not
// import (a module never imports a sibling). The full round trip — mint, mail,
// open, answer — is driven over the real HTTP stack in
// compose/integration/consent_integration_test.go, which is where the writer and
// this reader meet. What is proven HERE is the projection: given a row in a
// given state, what does the subject's package say about it.
//
// The seeded columns are exactly the ones the writer sets, so a change to which
// timestamps it records fails the schema check beside this rather than passing
// silently against a shape production never produces.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedConfirmLink writes one link in a named state. delivered_to and token_hash
// are set because the writer sets them and both are NOT NULL — this fixture
// carries the columns the export must NOT return, so a test asserting their
// absence is asking a real question rather than reading an empty row.
func seedConfirmLink(ctx context.Context, t *testing.T, owner *pgx.Conn, contact ids.ContactID,
	kind string, issued, expires time.Time, opened, consumed *time.Time,
) {
	t.Helper()
	// A consent link names the purpose it confirms and a record link never
	// does — confirm_token_purpose_is_consents. Honoured rather than worked
	// around: a fixture that broke it would be modelling a row the minting path
	// cannot produce.
	var purpose *ids.UUID
	if kind == "consent_confirmation" {
		purpose = seedConsentPurpose(ctx, t, owner)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO confirm_token
		    (contact_id, token_hash, delivered_to, issued_at, expires_at, opened_at, consumed_at,
		     kind, purpose_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		contact, "hash-"+kind+"-"+issued.Format(time.RFC3339Nano),
		"sara@sar.test", issued, expires, opened, consumed, kind, purpose); err != nil {
		t.Fatalf("seeding a %s confirm link: %v", kind, err)
	}
}

// seedConsentPurpose mints one purpose for a consent link to name. Each call
// makes its own, so two links in one fixture are never accidentally the same
// question asked twice.
func seedConsentPurpose(ctx context.Context, t *testing.T, owner *pgx.Conn) *ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(ctx,
		`INSERT INTO consent_purpose (id, key, label, requires_double_opt_in)
		 VALUES ($1, $2, 'Newsletter', true)`, id, "newsletter-"+id.String()); err != nil {
		t.Fatalf("seeding a consent purpose: %v", err)
	}
	return &id
}

func TestTheExportSaysWhatBecameOfEveryLinkItMailed(t *testing.T) {
	e := setupSARIdentifiers(t)
	now := time.Now()

	answered := now.Add(-2 * time.Hour)
	opened := now.Add(-3 * time.Hour)
	// One link per outcome, each in its own supersession scope.
	//
	// The writer supersedes per kind and per purpose, and so does the export, so
	// two links of one kind for one contact would make the older one superseded
	// rather than whatever this fixture meant it to be. Each row below therefore
	// gets its own kind, or its own purpose within a kind — which is also how
	// production produces four live links for one subject at once.
	//
	// The answered link is deliberately ALSO opened: a projection that tested
	// opened_at before consumed_at would call it merely opened, and this notices.
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "record_confirmation",
		now.Add(-4*time.Hour), now.Add(24*time.Hour), &opened, &answered)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "consent_confirmation",
		now.Add(-30*24*time.Hour), now.Add(-20*24*time.Hour), nil, nil)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "consent_confirmation",
		now.Add(-5*time.Hour), now.Add(48*time.Hour), &opened, nil)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "consent_confirmation",
		now.Add(-time.Hour), now.Add(72*time.Hour), nil, nil)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.ConsentLinks) != 4 {
		t.Fatalf("the export carries %d confirm link(s), want 4 — a subject cannot see a round "+
			"trip the package omits", len(pkg.ConsentLinks))
	}

	outcomes := map[string]int{}
	for _, link := range pkg.ConsentLinks {
		outcome, ok := link["outcome"].(string)
		if !ok {
			t.Fatalf("a confirm link reports no outcome: %v", link)
		}
		outcomes[outcome]++

		// The two columns the registration withholds must not be in the
		// package, whatever the outcome. Asserted on the ASSEMBLED row rather
		// than on the query text, which is the gate's question.
		for _, withheld := range []string{"token_hash", "delivered_to"} {
			if _, present := link[withheld]; present {
				t.Errorf("the export handed back %s (%v) — it is a live bearer credential, or the "+
					"address it was sent to, and an Art. 15 package assembled by an admin carries "+
					"neither", withheld, link[withheld])
			}
		}
	}

	for outcome, want := range map[string]int{
		"answered":            1,
		"expired_unanswered":  1,
		"opened_not_answered": 1,
		"awaiting_answer":     1,
	} {
		if outcomes[outcome] != want {
			t.Errorf("outcome %q appears %d time(s), want %d — the export is describing a link's "+
				"fate as something other than what happened to it", outcome, outcomes[outcome], want)
		}
	}
}

// A link the workspace itself retired is not one the subject ignored.
//
// Issuing a fresh link supersedes the pending one by setting its expires_at to
// the new link's issue time, so on the row it is indistinguishable from a lapse.
// Reporting that as "expired_unanswered" tells a subject they let something
// expire when the workspace withdrew it, which is a claim about their conduct
// and not true.
func TestASupersededLinkIsNotReportedAsIgnored(t *testing.T) {
	e := setupSARIdentifiers(t)
	now := time.Now()

	// Seeded exactly as the writer leaves them: the older row's expires_at IS
	// the newer row's issued_at, which is what makes the two cases identical
	// on the row and the reason this test exists.
	superseded := now.Add(-2 * time.Hour)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "record_confirmation",
		now.Add(-4*time.Hour), superseded, nil, nil)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "record_confirmation",
		superseded, now.Add(48*time.Hour), nil, nil)

	// A link of a DIFFERENT kind, expired on its own, is the control: the
	// writer supersedes per kind and per purpose, so this one must still read
	// as a genuine lapse. Without it a query that called everything superseded
	// would pass.
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "consent_confirmation",
		now.Add(-30*24*time.Hour), now.Add(-20*24*time.Hour), nil, nil)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	outcomes := map[string]int{}
	for _, link := range pkg.ConsentLinks {
		outcome, _ := link["outcome"].(string)
		outcomes[outcome]++
	}
	if outcomes["superseded_by_a_newer_link"] != 1 {
		t.Errorf("the retired link reads as superseded %d time(s), want 1 — the export is telling "+
			"the subject they ignored a link the workspace withdrew", outcomes["superseded_by_a_newer_link"])
	}
	if outcomes["expired_unanswered"] != 1 {
		t.Errorf("a genuinely lapsed link of another kind reads as expired %d time(s), want 1 — "+
			"supersession is per kind and per purpose, and this says the query lost that scope",
			outcomes["expired_unanswered"])
	}
	if outcomes["awaiting_answer"] != 1 {
		t.Errorf("the live replacement reads as awaiting %d time(s), want 1", outcomes["awaiting_answer"])
	}
}

// A subject who was never asked gets an empty section, not a missing one.
//
// The distinction matters to the reader: an absent key reads as "we have no
// idea", while an empty list says "we mailed you no links", which is a fact
// about their record and one they may well be checking.
func TestASubjectNeverAskedCarriesAnEmptyLinkSection(t *testing.T) {
	e := setupSARIdentifiers(t)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.ConsentLinks) != 0 {
		t.Fatalf("a subject who was mailed no link carries %d — the section is reading somebody "+
			"else's rows", len(pkg.ConsentLinks))
	}

	// Serialized, because len(nil) is also 0: the check above passes just as
	// well when the section was never assembled, which is the case this test
	// exists to tell apart. An absent key reads to the subject as "we have no
	// idea"; an empty list says "we mailed you none", and only one of those is
	// a fact about their record.
	body, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("serializing the package: %v", err)
	}
	if !strings.Contains(string(body), `"consent_links":[]`) {
		t.Error(`the package carries no "consent_links":[] — the section is missing rather than ` +
			"empty, so a subject who was never written to cannot tell that from an export that " +
			"forgot to ask")
	}
}

// One subject's links never appear in another's package.
func TestTheLinkSectionNeverCarriesAnotherSubjectsRoundTrip(t *testing.T) {
	e := setupSARIdentifiers(t)
	now := time.Now()

	stranger := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(e.ctx,
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Other Contact', 'manual', 'user:'||$2::text)`,
		stranger, ids.NewV7()); err != nil {
		t.Fatalf("seeding the stranger: %v", err)
	}
	seedConfirmLink(e.ctx, t, e.owner, stranger, "consent_confirmation",
		now.Add(-time.Hour), now.Add(72*time.Hour), nil, nil)
	seedConfirmLink(e.ctx, t, e.owner, e.contact, "record_confirmation",
		now.Add(-time.Hour), now.Add(72*time.Hour), nil, nil)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.ConsentLinks) != 1 {
		t.Fatalf("the export carries %d link(s) for a subject who was mailed one — the section is "+
			"not scoped to its subject", len(pkg.ConsentLinks))
	}
	if kind, _ := pkg.ConsentLinks[0]["kind"].(string); kind != "record_confirmation" {
		t.Errorf("the exported link is %q, want record_confirmation — this is the stranger's row", kind)
	}
}
