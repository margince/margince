// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Specs for qualify_lead's A15 contract: fill ONLY empty fields whose
// value is deterministically inferable (with the evidence that grounds
// it), report everything else as a gap — never overwrite, never guess.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/freemail"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/datasource"
)

// fakeSoR fakes the true provider boundary for the intent-tool specs:
// canned reads, recorded writes. Verbs a test never exercises stay on
// the embedded nil interface and would fail loudly if reached.
type fakeSoR struct {
	datasource.SystemOfRecordProvider
	records   map[datasource.EntityRef]datasource.Record
	updates   []datasource.UpdateInput
	creates   []datasource.CreateInput
	advances  []datasource.AdvanceDealInput
	createRef datasource.EntityRef
}

func (f *fakeSoR) Read(_ context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	rec, ok := f.records[ref]
	if !ok {
		return datasource.Record{}, apperrors.ErrNotFound
	}
	return rec, nil
}

func (f *fakeSoR) Update(_ context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	f.updates = append(f.updates, in)
	return in.Ref, nil
}

func (f *fakeSoR) Create(_ context.Context, in datasource.CreateInput) (datasource.EntityRef, error) {
	f.creates = append(f.creates, in)
	return f.createRef, nil
}

func (f *fakeSoR) AdvanceDeal(_ context.Context, in datasource.AdvanceDealInput) (datasource.EntityRef, error) {
	f.advances = append(f.advances, in)
	return datasource.EntityRef{Type: datasource.EntityDeal, ID: in.DealID}, nil
}

func leadFixture(t *testing.T, id ids.UUID, fields string, version int64) *fakeSoR {
	t.Helper()
	ref := datasource.EntityRef{Type: datasource.EntityLead, ID: id}
	return &fakeSoR{records: map[datasource.EntityRef]datasource.Record{
		ref: nativeRecord(datasource.Record{Ref: ref, Fields: json.RawMessage(fields), Version: version}),
	}}
}

// baselineConsumerMail is the platform matcher with NO deployment overlay —
// the same object compose builds, minus the workspace's administered rows,
// which is the honest stand-in for a unit test: it exercises the real list and
// the real walk rather than a map the test wrote itself. compose's own seam is
// exercised where it can reach a database.
type baselineConsumerMail struct{}

func (baselineConsumerMail) IsConsumer(_ context.Context, domain string) (bool, error) {
	return freemail.New(nil, nil).IsConsumer(domain), nil
}

type qualifyWire struct {
	RecordID string `json:"record_id"`
	Filled   map[string]struct {
		Value    string `json:"value"`
		Evidence []struct {
			Source  string `json:"source"`
			Snippet string `json:"snippet"`
		} `json:"evidence"`
	} `json:"filled"`
	Gaps []string `json:"gaps"`
}

func qualify(t *testing.T, p *fakeSoR, id ids.UUID) qualifyWire {
	t.Helper()
	raw, err := qualifyLead{p: p, consumerMail: baselineConsumerMail{}}.Handle(context.Background(),
		json.RawMessage(`{"record_id":"`+id.String()+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out qualifyWire
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestQualifyLeadFillsOnlyInferableEmptyFieldsWithEvidence(t *testing.T) {
	leadID := ids.NewV7()
	p := leadFixture(t, leadID,
		`{"email":"jane@acme-corp.io","full_name":"Jane Doe","company_name":"","title":null,"source":"webform"}`, 3)

	out := qualify(t, p, leadID)

	company, ok := out.Filled["company_name"]
	if !ok || company.Value != "Acme Corp" {
		t.Fatalf("filled = %+v, want company_name \"Acme Corp\" inferred from the email domain", out.Filled)
	}
	if len(company.Evidence) != 1 || company.Evidence[0].Source != "lead.email" || company.Evidence[0].Snippet != "jane@acme-corp.io" {
		t.Fatalf("company_name evidence = %+v, want the grounding email", company.Evidence)
	}
	if len(out.Filled) != 1 {
		t.Fatalf("filled %d fields, want exactly the one inferable gap: %+v", len(out.Filled), out.Filled)
	}
	if len(out.Gaps) != 1 || out.Gaps[0] != "title" {
		t.Fatalf("gaps = %v, want the one still-empty qualification field [title]", out.Gaps)
	}

	if len(p.updates) != 1 {
		t.Fatalf("provider saw %d updates, want 1", len(p.updates))
	}
	patchRaw, err := datasource.RawFields(p.updates[0].Patch)
	if err != nil {
		t.Fatal(err)
	}
	var patch map[string]string
	if err := json.Unmarshal(patchRaw, &patch); err != nil {
		t.Fatal(err)
	}
	if len(patch) != 1 || patch["company_name"] != "Acme Corp" {
		t.Fatalf("patch = %v — a fill-empty-only patch carries nothing but the filled field", patch)
	}
	if p.updates[0].IfVersion == nil || *p.updates[0].IfVersion != 3 {
		t.Fatalf("IfVersion = %v, want the version the fill was decided on (3)", p.updates[0].IfVersion)
	}
	if p.updates[0].Source != ToolSource {
		t.Fatalf("update source = %q, want the tool surface's provenance channel %q", p.updates[0].Source, ToolSource)
	}
}

func TestQualifyLeadReportsGapsInsteadOfGuessing(t *testing.T) {
	// A freemail domain names a mailbox host, not a company: nothing is
	// inferable, so nothing is written and the gap is surfaced.
	leadID := ids.NewV7()
	p := leadFixture(t, leadID,
		`{"email":"jane@gmail.com","full_name":"","company_name":"","title":"","source":"import"}`, 1)

	out := qualify(t, p, leadID)

	if len(out.Filled) != 0 {
		t.Fatalf("filled = %+v, want nothing — a freemail domain grounds no company", out.Filled)
	}
	if len(p.updates) != 0 {
		t.Fatalf("provider saw %d updates, want none when there is nothing evidenced to fill", len(p.updates))
	}
	want := []string{"full_name", "company_name", "title"}
	if len(out.Gaps) != len(want) {
		t.Fatalf("gaps = %v, want %v", out.Gaps, want)
	}
	for i, g := range want {
		if out.Gaps[i] != g {
			t.Fatalf("gaps = %v, want %v in the fixed qualification order", out.Gaps, want)
		}
	}
}

func TestQualifyLeadNeverOverwritesAnExistingValue(t *testing.T) {
	leadID := ids.NewV7()
	p := leadFixture(t, leadID,
		`{"email":"jane@acme-corp.io","full_name":"Jane Doe","company_name":"ACME Corporation GmbH","title":"CFO","source":"webform"}`, 7)

	out := qualify(t, p, leadID)

	if len(out.Filled) != 0 || len(p.updates) != 0 {
		t.Fatalf("filled=%+v updates=%d — a populated field is never touched, whatever the email would infer", out.Filled, len(p.updates))
	}
	if len(out.Gaps) != 0 {
		t.Fatalf("gaps = %v, want none for a fully qualified lead", out.Gaps)
	}
}

// The two doors used to answer one question differently, and both halves of
// the disagreement are pinned here.
//
// The list: this tool carried FIFTEEN domains against platform/freemail's
// 8,758-entry baseline, so the agent door happily created companies called
// "Zoho", "Yandex" and "Mail" from addresses the web door refuses to derive
// anything from at all. A wrong employer is worse than a reported gap — the
// gap asks a human, the employer is acted on.
func TestQualifyLeadRefusesEveryConsumerProviderTheWebDoorRefuses(t *testing.T) {
	// Each of these is in the platform baseline and was absent from the
	// fifteen-entry map: a provider the agent door used to name as an employer.
	for _, address := range []string{
		"jane@zoho.com",
		"jane@yandex.ru",
		"jane@mail.com",
		"jane@fastmail.com",
		"jane@tutanota.com",
		"jane@yahoo.co.uk",
	} {
		t.Run(address, func(t *testing.T) {
			leadID := ids.NewV7()
			p := leadFixture(t, leadID,
				`{"email":"`+address+`","full_name":"Jane","company_name":"","title":"x","source":"import"}`, 1)

			out := qualify(t, p, leadID)

			if _, filled := out.Filled["company_name"]; filled {
				t.Fatalf("%s: filled company_name = %+v — a mailbox host is not an employer",
					address, out.Filled["company_name"])
			}
			if len(p.updates) != 0 {
				t.Fatalf("%s: provider saw %d updates, want none", address, len(p.updates))
			}
			if len(out.Gaps) == 0 || out.Gaps[0] != "company_name" {
				t.Fatalf("%s: gaps = %v, want company_name reported rather than guessed", address, out.Gaps)
			}
		})
	}
}

// The derivation: cutting the domain at its FIRST dot named a subdomain.
// "eu.docusign.net" became "Eu", which is not a company and reads as a bug in
// front of whoever opens the record. The public-suffix walk names the
// registrable label, which is what a human reads as the company.
func TestQualifyLeadNamesTheCompanyNotTheSubdomain(t *testing.T) {
	for _, tc := range []struct{ email, want string }{
		{"jane@eu.docusign.net", "Docusign"},
		{"jane@mail.acme-corp.co.uk", "Acme Corp"},
		{"jane@acme.com", "Acme"},
		// An unknown TLD still derives cleanly — nothing legitimate is lost by
		// requiring the walk.
		{"jane@acme.internal", "Acme"},
	} {
		t.Run(tc.email, func(t *testing.T) {
			leadID := ids.NewV7()
			p := leadFixture(t, leadID,
				`{"email":"`+tc.email+`","full_name":"Jane","company_name":"","title":"x","source":"import"}`, 1)

			out := qualify(t, p, leadID)

			field, filled := out.Filled["company_name"]
			if !filled {
				t.Fatalf("%s: company_name not filled at all, want %q", tc.email, tc.want)
			}
			if field.Value != tc.want {
				t.Fatalf("%s -> %q, want %q", tc.email, field.Value, tc.want)
			}
		})
	}
}

// A lead's email is a string an outsider chose, and net/mail parses a domain
// far more loosely than DNS allows: `jane@%` is a legal RFC 5322 address whose
// domain is a LIKE wildcard. Nothing derivable comes out of one, and the tool
// reports the gap rather than writing a company named after a metacharacter.
func TestQualifyLeadDerivesNothingFromAnUnusableDomain(t *testing.T) {
	for _, address := range []string{"jane@%", "jane@", "jane", "jane@co.uk", "jane@-acme.com"} {
		t.Run(address, func(t *testing.T) {
			leadID := ids.NewV7()
			p := leadFixture(t, leadID,
				`{"email":"`+address+`","full_name":"Jane","company_name":"","title":"x","source":"import"}`, 1)

			out := qualify(t, p, leadID)

			if _, filled := out.Filled["company_name"]; filled {
				t.Fatalf("%s: filled company_name = %+v from an unusable domain",
					address, out.Filled["company_name"])
			}
			if len(p.updates) != 0 {
				t.Fatalf("%s: provider saw %d updates, want none", address, len(p.updates))
			}
		})
	}
}

// The seam's failure is the tool's failure. A matcher that cannot be read is
// not the same fact as "this domain is not a provider", and answering the
// second when the first happened is how a company gets derived from an address
// an operator had marked consumer — silently, and only while the database is
// unhappy.
func TestQualifyLeadRefusesRatherThanGuessingWhenTheListIsUnreadable(t *testing.T) {
	leadID := ids.NewV7()
	p := leadFixture(t, leadID,
		`{"email":"jane@acme.com","full_name":"Jane","company_name":"","title":"x","source":"import"}`, 1)

	_, err := qualifyLead{p: p, consumerMail: brokenConsumerMail{}}.Handle(context.Background(),
		json.RawMessage(`{"record_id":"`+leadID.String()+`"}`))

	if err == nil {
		t.Fatal("qualify answered while the consumer-mail list was unreadable — a fill decided on an unknown is a guess")
	}
	if len(p.updates) != 0 {
		t.Fatalf("provider saw %d updates, want none", len(p.updates))
	}
}

type brokenConsumerMail struct{}

func (brokenConsumerMail) IsConsumer(context.Context, string) (bool, error) {
	return false, errors.New("the consumer-mail overlay is unreadable")
}

// A registry built without the seam cannot answer the consumer-mail question,
// and answering it anyway would derive a company from an address an operator
// may have marked consumer. `RegisterCoreTools` takes the seam as a plain
// interface and thirteen call sites in this tree pass nil for the seams they do
// not exercise, so an unwired one is reachable — it refuses on the same terms
// an unreadable list refuses on rather than nil-panicking at the first lead
// with an email.
func TestQualifyLeadRefusesWhenNoConsumerMailListIsWired(t *testing.T) {
	leadID := ids.NewV7()
	p := leadFixture(t, leadID,
		`{"email":"jane@acme.com","full_name":"Jane","company_name":"","title":"x","source":"import"}`, 1)

	_, err := qualifyLead{p: p}.Handle(context.Background(),
		json.RawMessage(`{"record_id":"`+leadID.String()+`"}`))

	if err == nil {
		t.Fatal("qualify answered with no consumer-mail list wired at all")
	}
	if len(p.updates) != 0 {
		t.Fatalf("provider saw %d updates, want none", len(p.updates))
	}
}
