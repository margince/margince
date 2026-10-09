// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// attach_document and list_documents through the registry the api role
// composes, as an agent acting for a rep, against real Postgres.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// minimalPDF is a complete one-page PDF a reader opens.
const minimalPDF = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n" +
	"xref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000052 00000 n \n0000000101 00000 n \n" +
	"trailer<</Size 4/Root 1 0 R>>\nstartxref\n164\n%%EOF\n"

// documentAgentLimit is the operator's attachment ceiling for these tests, small
// enough that an oversize file costs nothing to build.
const documentAgentLimit = 4 << 10

// documentSeat stands in for the identity resolver with fixed grants: the
// harness seeds no role assignments, so the live resolver would deny everything.
type documentSeat struct {
	perms principal.Permissions
	teams []ids.UUID
}

func (s documentSeat) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{Permissions: s.perms, TeamIDs: s.teams}, nil
}

func (documentSeat) SeatType(context.Context, ids.UUID, ids.UUID) (principal.SeatType, error) {
	return principal.SeatFull, nil
}

func (s documentSeat) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, s.EffectiveRBAC, s.SeatType)
}

// documentRepPerms is a team-scoped rep who may change every record a document hangs off.
func documentRepPerms(update bool) principal.Permissions {
	grant := principal.ObjectGrant{Create: update, Read: true, Update: update}
	return principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"company": grant, "contact": grant, "lead": grant, "deal": grant, "project": grant,
			"activity": {Read: true}, "pipeline": {Read: true}, "installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	}
}

type documentHarness struct {
	e        *integration.Env
	blob     blobstore.Store
	registry *agents.Registry
	agent    context.Context
	passport ids.UUID
}

func newDocumentHarness(t *testing.T, perms principal.Permissions) documentHarness {
	t.Helper()
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	limits := deployconfig.Config{}.EffectiveUploads()
	limits.Attachment = documentAgentLimit
	srv := &Server{blob: blob, uploadLimits: limits}
	seat := documentSeat{perms: perms, teams: []ids.UUID{e.Team1}}
	registry := registryWithGate(e.DB(), auth.NewGate(seat), nil, SendPath{}, companyEnricher{},
		nil, nil, nil, nil, nil, registryFeatures{served: srv})
	passport := e.SeedPassport(t, integration.OwnerConn(t), "document agent")
	agent := identity.AgentIdentity{
		PassportID:  ids.From[ids.PassportKind](passport),
		WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		OnBehalfOf:  ids.From[ids.UserKind](e.Rep1),
		SeatType:    string(principal.SeatFull),
		Scopes:      principal.ScopeSet{principal.ScopeRead: {}, principal.ScopeWrite: {}},
		Teams:       []ids.TeamID{ids.From[ids.TeamKind](e.Team1)},
		Permissions: perms,
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return documentHarness{
		e: e, blob: blob, registry: registry, passport: passport,
		agent: principal.WithActor(ctx, agent.Principal()),
	}
}

// createAs makes a record through create_record as a human, so the fixture is a
// row production writes.
func (h documentHarness) createAs(t *testing.T, owner ids.UUID, args string) ids.UUID {
	t.Helper()
	human := h.e.As(owner, []ids.UUID{h.e.Team1}, integration.AdminPerms)
	out, err := h.registry.Invoke(human, "create_record", json.RawMessage(args))
	if err != nil {
		t.Fatalf("create_record(%s): %v", args, err)
	}
	var created struct {
		Data struct {
			ID ids.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unreadable create_record answer %s: %v", out, err)
	}
	return created.Data.ID
}

// callTool invokes a tool as the agent and holds a success to the tool's own schema.
func callTool[T any](t *testing.T, h documentHarness, tool string, args map[string]any) (T, error) {
	t.Helper()
	var out T
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding %s arguments: %v", tool, err)
	}
	answer, err := h.registry.Invoke(h.agent, tool, raw)
	if err != nil {
		return out, err
	}
	spec, _ := h.registry.Spec(tool)
	if defect := agents.ResultDefect(spec.OutputSchema, answer); defect != "" {
		t.Errorf("%s answered %s, outside its own schema: %s", tool, answer, defect)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(answer, &envelope); err != nil {
		t.Fatalf("%s answered no envelope: %v\n%s", tool, err, answer)
	}
	if err := json.Unmarshal(envelope.Data, &out); err != nil {
		t.Fatalf("%s payload does not decode: %v\n%s", tool, err, envelope.Data)
	}
	return out, nil
}

func (h documentHarness) attach(t *testing.T, args map[string]any) (agents.AttachedDocument, error) {
	t.Helper()
	return callTool[agents.AttachedDocument](t, h, "attach_document", args)
}

func (h documentHarness) list(t *testing.T, entityType string, id ids.UUID) (agents.DocumentPage, error) {
	t.Helper()
	return callTool[agents.DocumentPage](t, h, "list_documents",
		map[string]any{"entity_type": entityType, "entity_id": id.String()})
}

func attachArgs(entityType string, entityID ids.UUID, filename, contentType string, content []byte) map[string]any {
	return map[string]any{
		"entity_type": entityType, "entity_id": entityID.String(), "filename": filename,
		"content_type": contentType, "content_base64": base64.StdEncoding.EncodeToString(content),
	}
}

func (h documentHarness) attachmentRows(t *testing.T, entityID ids.UUID) int {
	t.Helper()
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM attachment WHERE entity_id = $1`, entityID).Scan(&n); err != nil {
		t.Fatalf("counting attachments: %v", err)
	}
	return n
}

func (h documentHarness) storedBytes(t *testing.T, attachment ids.UUID) []byte {
	t.Helper()
	human := h.e.As(h.e.Rep1, []ids.UUID{h.e.Team1}, integration.AdminPerms)
	_, body, err := activities.NewStore(h.e.DB()).WithBlobstore(h.blob).OpenAttachment(human, attachment)
	if err != nil {
		t.Fatalf("opening attachment %s: %v", attachment, err)
	}
	defer func() {
		if err := body.Close(); err != nil {
			t.Errorf("closing attachment %s: %v", attachment, err)
		}
	}()
	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("reading attachment %s: %v", attachment, err)
	}
	return content
}

func TestAnAgentAttachesAPDFToEveryRecordKindAndListsItWhereTheTabDoes(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	pipeline, open, _ := integration.DealFixture(t, h.e)
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Paper GmbH"}}`)
	records := map[string]ids.UUID{
		"company": company,
		"contact": h.createAs(t, h.e.Rep1, `{"record_type":"contact","fields":{"full_name":"Paula Paper"}}`),
		// A human's lead arrives unassigned, and an unowned row is nobody's to change.
		"lead": h.createAs(t, h.e.Rep1, `{"record_type":"lead","fields":{"email":"lead@paper.example","owner_id":"`+
			h.e.Rep1.String()+`"}}`),
		// On the company, so the company library holds a file its own attachments do not.
		"deal": h.createAs(t, h.e.Rep1, `{"record_type":"deal","fields":{"name":"Paper renewal","pipeline_id":"`+
			pipeline.String()+`","stage_id":"`+open.String()+`","company_id":"`+company.String()+`"}}`),
		"project": h.createAs(t, h.e.Rep1, `{"record_type":"project","fields":{"name":"Paper rollout","company_id":"`+
			company.String()+`"}}`),
	}
	sum := sha256.Sum256([]byte(minimalPDF))
	for entityType, id := range records {
		t.Run(entityType, func(t *testing.T) {
			stored, err := h.attach(t, attachArgs(entityType, id, "terms.pdf", "application/pdf", []byte(minimalPDF)))
			if err != nil {
				t.Fatalf("attach_document on a %s: %v", entityType, err)
			}
			if stored.Checksum != hex.EncodeToString(sum[:]) {
				t.Errorf("the stored checksum %q is not the source's", stored.Checksum)
			}
			if got := h.storedBytes(t, stored.AttachmentID); string(got) != minimalPDF {
				t.Errorf("the stored bytes differ from the file attached (%d bytes, want %d)", len(got), len(minimalPDF))
			}
			if want := "agent:" + h.passport.String(); stored.CapturedBy != want {
				t.Errorf("captured_by = %q, want the passport principal %q", stored.CapturedBy, want)
			}
			page, err := h.list(t, entityType, id)
			if err != nil {
				t.Fatalf("list_documents on a %s: %v", entityType, err)
			}
			if !listsDocument(page.Documents, stored.AttachmentID) {
				t.Errorf("list_documents on the %s does not show the file just attached: %+v", entityType, page.Documents)
			}
		})
	}
	h.assertTheTabsListWhatTheToolAttached(t, records["company"], records["deal"])
}

func listsDocument(docs []agents.AttachedDocument, id ids.UUID) bool {
	for _, doc := range docs {
		if doc.AttachmentID == id {
			return true
		}
	}
	return false
}

// The company and deal tabs read their own libraries; what the tool attached is in both.
func (h documentHarness) assertTheTabsListWhatTheToolAttached(t *testing.T, company, deal ids.UUID) {
	t.Helper()
	human := h.e.As(h.e.Rep1, []ids.UUID{h.e.Team1}, integration.AdminPerms)
	store := activities.NewStore(h.e.DB())
	library, _, err := store.ListCompanyDocuments(human, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("reading the company library: %v", err)
	}
	dealFiles, _, err := store.ListDealDocuments(human, deal, activities.DealDocumentFilters{})
	if err != nil {
		t.Fatalf("reading the deal's files: %v", err)
	}
	if len(library) == 0 || len(dealFiles) == 0 {
		t.Fatalf("a tab shows nothing: company library %d rows, deal files %d", len(library), len(dealFiles))
	}
	fromTool, err := h.list(t, "company", company)
	if err != nil {
		t.Fatalf("list_documents on the company: %v", err)
	}
	if len(library) < 2 || len(fromTool.Documents) != len(library) {
		t.Errorf("list_documents shows %d of the company library's %d files, want the deal's file too",
			len(fromTool.Documents), len(library))
	}
}

func TestAnAgentWhoseHumanMayNotChangeTheRecordIsRefusedAndNothingIsStored(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(false))
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Read Only AG"}}`)
	_, err := h.attach(t, attachArgs("company", company, "terms.pdf", "application/pdf", []byte(minimalPDF)))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("attaching without update on the company = %v, want permission denied", err)
	}
	if n := h.attachmentRows(t, company); n != 0 {
		t.Errorf("a refused attach left %d rows", n)
	}
}

// Every record type reads workspace-wide, so the record hidden here is a
// captured contact still private to the rep whose mailbox found it.
func TestARecordTheAgentCannotSeeReadsAsMissing(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	hidden := h.createAs(t, h.e.Rep3, `{"record_type":"contact","fields":{"full_name":"Private Capture"}}`)
	h.e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, h.e.Rep3)
	_, err := h.attach(t, attachArgs("contact", hidden, "terms.pdf", "application/pdf", []byte(minimalPDF)))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("attaching to a contact private to another rep = %v, want not found", err)
	}
	_, err = h.list(t, "contact", hidden)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("listing a contact private to another rep = %v, want not found", err)
	}
	if n := h.attachmentRows(t, hidden); n != 0 {
		t.Errorf("a refused attach left %d rows", n)
	}
}

// A deal another team owns is readable and not the rep's to change, so its files are not either.
func TestARecordAnotherTeamOwnsIsRefusedOnAuthority(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	pipeline, open, _ := integration.DealFixture(t, h.e)
	theirs := h.createAs(t, h.e.Rep3, `{"record_type":"deal","fields":{"name":"Other team's renewal","pipeline_id":"`+
		pipeline.String()+`","stage_id":"`+open.String()+`"}}`)
	_, err := h.attach(t, attachArgs("deal", theirs, "terms.pdf", "application/pdf", []byte(minimalPDF)))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("attaching to another team's deal = %v, want permission denied", err)
	}
	if n := h.attachmentRows(t, theirs); n != 0 {
		t.Errorf("a refused attach left %d rows", n)
	}
}

func TestAnSVGAndAnOversizeFileAreRefusedAndNothingIsStored(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Refusals Inc"}}`)
	_, err := h.attach(t, attachArgs("company", company, "logo.svg", "image/svg+xml", []byte("<svg/>")))
	fault, classified := httperr.Classify(err)
	if !classified || len(fault.Fields) == 0 || fault.Fields[0].Code != "unsupported_file_type" {
		t.Errorf("attaching an .svg = %v, want an unsupported_file_type refusal", err)
	}
	oversize := make([]byte, documentAgentLimit+1)
	_, err = h.attach(t, attachArgs("company", company, "big.pdf", "application/pdf", oversize))
	var badArgs *agents.BadArgsError
	if !errors.As(err, &badArgs) {
		t.Errorf("attaching a file over the operator's limit = %v, want an argument refusal", err)
	}
	if n := h.attachmentRows(t, company); n != 0 {
		t.Errorf("refused attaches left %d rows", n)
	}
}

func TestARetriedAttachUnderOneKeyStoresOneFile(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Retry GmbH"}}`)
	args := attachArgs("company", company, "terms.pdf", "application/pdf", []byte(minimalPDF))
	args["idempotency_key"] = "attach-terms-once"
	first, err := h.attach(t, args)
	if err != nil {
		t.Fatalf("first attach: %v", err)
	}
	again, err := h.attach(t, args)
	if err != nil {
		t.Fatalf("retried attach: %v", err)
	}
	if again.AttachmentID != first.AttachmentID {
		t.Errorf("the retry answered attachment %s, want the first call's %s", again.AttachmentID, first.AttachmentID)
	}
	if n := h.attachmentRows(t, company); n != 1 {
		t.Errorf("a retried attach under one key left %d rows, want 1", n)
	}
}

// A retry's answer is a receipt for the write that already happened, so it is
// replayed even after a human removed the file: only the parent is re-proved.
func TestARetriedAttachReplaysItsReceiptAfterTheFileIsRemoved(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Receipt GmbH"}}`)
	args := attachArgs("company", company, "terms.pdf", "application/pdf", []byte(minimalPDF))
	args["idempotency_key"] = "attach-then-remove"
	first, err := h.attach(t, args)
	if err != nil {
		t.Fatalf("first attach: %v", err)
	}
	human := h.e.As(h.e.Rep1, []ids.UUID{h.e.Team1}, integration.AdminPerms)
	if err := activities.NewStore(h.e.DB()).ArchiveAttachment(human, first.AttachmentID); err != nil {
		t.Fatalf("removing the file: %v", err)
	}
	again, err := h.attach(t, args)
	if err != nil {
		t.Fatalf("the retry after the file was removed: %v", err)
	}
	if again.AttachmentID != first.AttachmentID {
		t.Errorf("the retry answered attachment %s, want the first call's %s", again.AttachmentID, first.AttachmentID)
	}
	if n := h.attachmentRows(t, company); n != 1 {
		t.Errorf("the retry left %d rows, want the one first stored", n)
	}
}

// An archived record stays readable by id, and so do its files: list_documents
// answers what the record's Documents tab shows, on either kind of tab read.
func TestAnArchivedRecordsDocumentsListAsItsTabShowsThem(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	company := h.createAs(t, h.e.Rep1, `{"record_type":"company","fields":{"display_name":"Retired AG"}}`)
	contact := h.createAs(t, h.e.Rep1, `{"record_type":"contact","fields":{"full_name":"Rhea Retired"}}`)
	for entityType, id := range map[string]ids.UUID{"company": company, "contact": contact} {
		if _, err := h.attach(t, attachArgs(entityType, id, "terms.pdf", "application/pdf", []byte(minimalPDF))); err != nil {
			t.Fatalf("attaching to the %s: %v", entityType, err)
		}
	}
	human := h.e.As(h.e.Rep1, []ids.UUID{h.e.Team1}, integration.AdminPerms)
	directory := contacts.NewStore(h.e.DB())
	if _, err := directory.ArchiveCompany(human, ids.From[ids.CompanyKind](company), nil); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}
	if _, err := directory.ArchiveContact(human, ids.From[ids.ContactKind](contact), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}

	store := activities.NewStore(h.e.DB())
	companyTab, _, err := store.ListCompanyDocuments(human, company, activities.DocumentFilters{})
	if err != nil {
		t.Fatalf("the archived company's tab: %v", err)
	}
	contactTab, _, err := store.ListAttachments(human, "contact", contact, nil, nil)
	if err != nil {
		t.Fatalf("the archived contact's tab: %v", err)
	}
	for entityType, tc := range map[string]struct {
		id  ids.UUID
		tab int
	}{"company": {company, len(companyTab)}, "contact": {contact, len(contactTab)}} {
		page, err := h.list(t, entityType, tc.id)
		if err != nil {
			t.Errorf("list_documents on the archived %s: %v", entityType, err)
			continue
		}
		if tc.tab == 0 || len(page.Documents) != tc.tab {
			t.Errorf("list_documents on the archived %s shows %d files, its tab %d; want the same, and the file",
				entityType, len(page.Documents), tc.tab)
		}
	}
}

// A deal with more files than one page walks to the end on next_cursor, each file once.
func TestListDocumentsWalksADealsFilesPageByPage(t *testing.T) {
	h := newDocumentHarness(t, documentRepPerms(true))
	pipeline, open, _ := integration.DealFixture(t, h.e)
	deal := h.createAs(t, h.e.Rep1, `{"record_type":"deal","fields":{"name":"Paged renewal","pipeline_id":"`+
		pipeline.String()+`","stage_id":"`+open.String()+`"}}`)
	want := map[ids.UUID]int{}
	for _, name := range []string{"terms.pdf", "annex.pdf"} {
		stored, err := h.attach(t, attachArgs("deal", deal, name, "application/pdf", []byte(minimalPDF)))
		if err != nil {
			t.Fatalf("attaching %s: %v", name, err)
		}
		want[stored.AttachmentID] = 0
	}
	args := map[string]any{"entity_type": "deal", "entity_id": deal.String(), "limit": 1}
	pages := 0
	for ; pages < len(want)+1; pages++ {
		page, err := callTool[agents.DocumentPage](t, h, "list_documents", args)
		if err != nil {
			t.Fatalf("page %d: %v", pages+1, err)
		}
		for _, doc := range page.Documents {
			want[doc.AttachmentID]++
		}
		if page.NextCursor == "" {
			break
		}
		args["cursor"] = page.NextCursor
	}
	if pages+1 != len(want) {
		t.Errorf("the walk took %d pages, want %d", pages+1, len(want))
	}
	for id, seen := range want {
		if seen != 1 {
			t.Errorf("file %s was listed %d times, want once", id, seen)
		}
	}
}

func TestWithNoObjectStoreAnAttachIsRefusedAsNotImplemented(t *testing.T) {
	e := integration.Setup(t)
	registry := registryWithGate(e.DB(), auth.NewGate(adminSeat{}), nil, SendPath{}, companyEnricher{},
		nil, nil, nil, nil, nil, registryFeatures{})
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	out, err := registry.Invoke(human, "create_record", json.RawMessage(`{"record_type":"company","fields":{"display_name":"No Store"}}`))
	if err != nil {
		t.Fatalf("create_record: %v", err)
	}
	var created struct {
		Data struct {
			ID ids.UUID `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unreadable create_record answer: %v", err)
	}
	raw, err := json.Marshal(attachArgs("company", created.Data.ID, "terms.pdf", "application/pdf", []byte(minimalPDF)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Invoke(human, "attach_document", raw)
	if fault, ok := httperr.Classify(err); !ok || fault.Status != 501 {
		t.Errorf("attaching with no object store = %v, want the 501 the REST upload answers", err)
	}
}
