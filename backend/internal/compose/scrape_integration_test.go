// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// scrapeCompany (EP05): the enrich verb on a KNOWN company. The shared no-guess
// gate keeps only evidence-grounded fields, the surviving fields stage a 🟡
// approval BOUND to the company (nothing touches the record), a company the caller
// cannot see is existence-hidden (404), and acceptance fills only the company's
// empty fields as agent:scrape — exactly once.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var scrapePerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company": {Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

// acmeExtraction is the model reply used across the scrape tests: two fields
// whose evidence is verbatim in acmePage survive; a hallucinated snippet, an
// out-of-range confidence and an unknown field name are dropped.
const acmeExtraction = `{"fields":[
	{"field":"value_proposition","value":"Fast onboarding","evidence_snippet":"Onboard your team in minutes, not weeks","confidence":0.9},
	{"field":"icp","value":"RevOps at SaaS scale-ups","evidence_snippet":"Built for RevOps leaders at scaling SaaS companies","confidence":0.7},
	{"field":"legal_name","value":"Acme GmbH","evidence_snippet":"this text is NOT on the page","confidence":0.9},
	{"field":"industry","value":"Software","evidence_snippet":"Acme GmbH","confidence":1.7},
	{"field":"made_up_field","value":"x","evidence_snippet":"Acme GmbH","confidence":0.5}]}`

// insertCompany creates a company owned by owner, optionally with a domain and a
// human-set industry, and returns its id.
func insertCompany(t *testing.T, e *integration.Env, owner ids.UUID, domain, industry string) ids.UUID {
	t.Helper()
	companyID := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO company (id, owner_id, display_name, industry, source, captured_by)
			VALUES ($1, $2, 'Acme', NULLIF($3,''), 'manual', 'human:owner')`,
			companyID, owner, industry); err != nil {
			return err
		}
		if domain == "" {
			return nil
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:owner')`, companyID, domain)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return companyID
}

func TestScrapeStagesEnrichmentBoundToCompany(t *testing.T) {
	e := integration.Setup(t)
	companyID := insertCompany(t, e, e.Rep1, "acme.example", "")
	fake := ai.NewFakeClient().Script(acmeExtraction)
	engine := &scrapeEngine{extract: evidenceExtractor{fetch: acmePage, brain: fakeModelPath(t, fake).ColdStart}, contacts: e.Contacts, approvals: approvals.NewService(e.DB())}

	proposal, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), companyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if ids.UUID(proposal.CompanyId) != companyID {
		t.Fatalf("proposal bound to %s, want the target company %s", ids.UUID(proposal.CompanyId), companyID)
	}
	if proposal.SourceUrl != "https://acme.example" {
		t.Fatalf("source url = %q, want the company's own domain", proposal.SourceUrl)
	}
	if len(proposal.Fields) != 2 {
		t.Fatalf("gate let %d fields through, want 2 (hallucinated evidence, bad confidence, unknown name drop): %+v", len(proposal.Fields), proposal.Fields)
	}
	if proposal.Status != "staged" || proposal.ProposalId.String() == ids.Nil.String() {
		t.Fatalf("proposal not staged: %+v", proposal)
	}

	// The staged row is bound to the company and emitted the enrichment event.
	var kind, status, targetType string
	var targetID ids.UUID
	var eventCount int
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT kind, status, coalesce(target_entity_type,''), coalesce(target_entity_id, $2)
			 FROM approval WHERE id = $1`, ids.From[ids.ApprovalKind](ids.UUID(proposal.ProposalId)), ids.Nil).Scan(&kind, &status, &targetType, &targetID); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'approval.requested'`).Scan(&eventCount)
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "enrich" || status != "pending" || eventCount != 1 {
		t.Fatalf("staging landed kind=%s status=%s approval.requested=%d, want enrich/pending/1", kind, status, eventCount)
	}
	if targetType != "company" || targetID != companyID {
		t.Fatalf("approval not bound to the company (target %s/%s)", targetType, targetID)
	}
}

func TestScrapeHidesAnInvisibleCompany(t *testing.T) {
	e := integration.Setup(t)
	// Capture-private to rep3 (team2): a company is otherwise readable by
	// every seat, so visibility='owner' is what makes it invisible to rep1.
	hidden := insertCompany(t, e, e.Rep3, "hidden.example", "")
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	fake := ai.NewFakeClient().Script(acmeExtraction)
	engine := &scrapeEngine{extract: evidenceExtractor{fetch: acmePage, brain: fakeModelPath(t, fake).ColdStart}, contacts: e.Contacts, approvals: approvals.NewService(e.DB())}

	// Both the domain path and the override path must 404 a company the caller
	// cannot see — existence-hiding, before any egress on their behalf.
	if _, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), hidden, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("enrich a hidden company (domain path) → %v, want ErrNotFound", err)
	}
	if _, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), hidden, "https://attacker.example"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("enrich a hidden company (override path) → %v, want ErrNotFound", err)
	}
	// A never-existed id is 404 too (same EnsureVisible path).
	if _, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), ids.NewV7(), ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("enrich a nonexistent company → %v, want ErrNotFound", err)
	}
}

func TestScrapeDegradesHonestly(t *testing.T) {
	e := integration.Setup(t)
	// (a) A visible company with a domain but nothing survives the gate → unreadable.
	companyID := insertCompany(t, e, e.Rep1, "acme.example", "")
	allHallucinated := ai.NewFakeClient().Script(
		`{"fields":[{"field":"icp","value":"guessed","evidence_snippet":"nowhere on the page","confidence":0.9}]}`)
	engine := &scrapeEngine{extract: evidenceExtractor{fetch: acmePage, brain: fakeModelPath(t, allHallucinated).ColdStart}, contacts: e.Contacts, approvals: approvals.NewService(e.DB())}
	var unreadable *unreadableError
	if _, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), companyID, ""); !errors.As(err, &unreadable) {
		t.Fatalf("all-hallucinated extraction → %v, want unreadable", err)
	}

	// (b) A visible company with NO domain and no override → no target to read.
	noDomain := insertCompany(t, e, e.Rep1, "", "")
	if _, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), noDomain, ""); !errors.Is(err, contacts.ErrNoEnrichTarget) {
		t.Fatalf("company without a domain → %v, want ErrNoEnrichTarget", err)
	}
}

func TestScrapeAcceptFillsOnlyEmptyFields(t *testing.T) {
	e := integration.Setup(t)
	// Human already set the industry; legal_name is empty.
	companyID := insertCompany(t, e, e.Rep1, "acme.example", "Handcrafted Industry")
	fake := ai.NewFakeClient().Script(acmeExtraction, acmeExtraction)

	svc := approvals.NewService(e.DB())
	svc.WithEffect("enrich", scrapeAcceptEffect(svc, e.Contacts))
	engine := &scrapeEngine{extract: evidenceExtractor{fetch: acmePage, brain: fakeModelPath(t, fake).ColdStart}, contacts: e.Contacts, approvals: svc}

	proposal, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), companyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), ids.From[ids.ApprovalKind](ids.UUID(proposal.ProposalId)), true, nil); err != nil {
		t.Fatalf("accept: %v", err)
	}

	var industry, capturedBy, source string
	var profileRows, companies int
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT industry FROM company WHERE id = $1`, companyID).Scan(&industry); err != nil {
			return err
		}
		if err := tx.QueryRow(context.Background(), `SELECT count(*) FROM company`).Scan(&companies); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(),
			`SELECT count(*), max(captured_by), max(source) FROM company_profile_field WHERE company_id = $1`,
			companyID).Scan(&profileRows, &capturedBy, &source)
	})
	if err != nil {
		t.Fatal(err)
	}
	if companies != 1 {
		t.Fatalf("enrichment created a duplicate company (%d rows) instead of targeting the named one", companies)
	}
	if industry != "Handcrafted Industry" {
		t.Fatalf("accept OVERWROTE a human-set industry: %q", industry)
	}
	if profileRows != 2 || capturedBy != "agent:scrape" {
		t.Fatalf("evidence rows = %d captured_by=%q, want 2 as agent:scrape", profileRows, capturedBy)
	}
	// All accepted website evidence uses the one site_read source vocabulary;
	// captured_by and the source URL identify the executing agent and dossier.
	if source != "site_read" {
		t.Fatalf("evidence source = %q, want site_read", source)
	}

	// Exactly-once: the approval is consumed and a re-decide is refused.
	var already *approvals.AlreadyDecidedError
	if _, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), ids.From[ids.ApprovalKind](ids.UUID(proposal.ProposalId)), true, nil); !errors.As(err, &already) {
		t.Fatalf("re-decide → %v, want AlreadyDecided", err)
	}

	// A REJECTED enrichment writes nothing.
	proposal2, err := engine.Propose(e.As(e.Rep1, []ids.UUID{e.Team1}, scrapePerms), companyID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decide(e.As(e.Rep2, nil, integration.AdminPerms), ids.From[ids.ApprovalKind](ids.UUID(proposal2.ProposalId)), false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}
	var rejectedRows int
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM company_profile_field WHERE company_id = $1`, companyID).Scan(&rejectedRows)
	})
	if err != nil || rejectedRows != 2 {
		t.Fatalf("reject changed the profile rows to %d (err=%v), want the 2 from the accepted proposal", rejectedRows, err)
	}
}
