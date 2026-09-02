// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Art. 15 package an access request asks for: who may assemble one, which
// requests have one, and what it has to contain.
//
// Before this the queue could record an access request and an officer could mark
// it fulfilled with no product path that produced anything, so "fulfilled" meant
// somebody had said so. These tests are about the half that makes the word true.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTheAccessPackageCarriesWhatIsHeldAboutTheSubject(t *testing.T) {
	e := Setup(t)
	person := seedSubjectWithMail(t, e)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("assembling the package: %v", err)
	}
	if len(pkg.Subject) == 0 {
		t.Error("the package names no subject")
	}
	if len(pkg.Emails) == 0 {
		t.Error("the package carries no address for a subject whose address is on file")
	}
	// The correspondence, not only the identifiers. An export that hands back a
	// name and no messages answers a question the subject did not ask.
	if len(pkg.Activities) == 0 {
		t.Error("the package carries no activities for a subject with mail on their timeline")
	}
}

func TestAssemblingAPackageNeedsTheTrustErasureNeeds(t *testing.T) {
	// AssembleSAR's own two conditions: the person.delete grant, and an
	// unbounded row scope because Art. 15 owes the subject everything held
	// rather than the slice one colleague may see.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)

	// read_only holds no person.delete and is refused by the grant.
	readOnly := e.As(ids.NewV7(), []ids.UUID{e.Team1}, ReadOnlyPerms)
	if _, err := privacy.AssembleSAR(readOnly, e.DB(), ids.From[ids.PersonKind](person)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("read_only assembled a subject-access package: err=%v, want permission denied", err)
	}
	// A bounded rep is refused twice over — no grant, and no scope.
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if _, err := privacy.AssembleSAR(repCtx, e.DB(), ids.From[ids.PersonKind](person)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a bounded rep assembled a subject-access package: err=%v, want permission denied", err)
	}
}

func TestTheQueueGateIsWhatKeepsTheExportAdminOnly(t *testing.T) {
	// The gap this route must not open. AssembleSAR admits any unbounded human
	// holding person.delete, and the seeded defaults give that to ops and
	// management as well as admin — so the assembler ALONE is more open than the
	// queue that owns this workflow.
	//
	// What closes it is reading the request first: GetDSR takes requireDSRAdmin,
	// which is admin-only, so a handler that reaches the request before it
	// assembles anything is gated by the narrower of the two. This pins both
	// halves, because a future handler that assembled from a person id in the
	// path instead would silently be reachable by two more roles.
	e := Setup(t)
	store := consent.NewStore(e.DB())
	person := seedSubjectWithMail(t, e)
	created, err := store.CreateDSR(e.Admin(), consent.CreateDSRInput{
		Kind:       "access",
		SubjectRef: person.String(),
		DueAt:      time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seeding an access request: %v", err)
	}

	opsCtx := e.As(ids.NewV7(), []ids.UUID{e.Team1}, OpsPerms)
	// The assembler admits ops: this is the fact the route has to defend against,
	// not a defect in AssembleSAR — its own contract is the erasure trust level.
	if _, err := privacy.AssembleSAR(opsCtx, e.DB(), ids.From[ids.PersonKind](person)); err != nil {
		t.Fatalf("ops is refused by the assembler itself, so this test no longer describes the "+
			"reason the queue gate matters: %v", err)
	}
	// And the queue refuses them, which is what the route reads first.
	if _, err := store.GetDSR(opsCtx, created.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("ops read a subject request: err=%v, want permission denied — the queue gate is "+
			"the only thing keeping the Art. 15 export admin-only", err)
	}
}

func TestAnAgentIsRefusedTheAccessPackageWhateverItsPassportCarries(t *testing.T) {
	// The arm the object grant cannot express. An agent acting under a passport
	// carries the granting human's live grants, so an admin's read-scoped
	// passport would otherwise assemble a subject's entire record.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)

	agent := principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:sdr",
		SeatType:    principal.SeatFull,
		Permissions: AdminPerms,
	}
	ctx := principal.WithActor(
		principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()),
		agent)
	if _, err := privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.PersonKind](person)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent carrying an admin's grants assembled a package: err=%v, want permission denied", err)
	}
}

func TestOnlyAnAccessRequestHasAPackage(t *testing.T) {
	// An erasure and a rectification are answered by their own paths. Handing
	// back a subject's whole record to close a request that asked for something
	// else is the export nobody asked for.
	e := Setup(t)
	store := consent.NewStore(e.DB())
	person := seedSubjectWithMail(t, e)

	for _, kind := range []string{"erasure", "rectify"} {
		created, err := store.CreateDSR(e.Admin(), consent.CreateDSRInput{
			Kind:       kind,
			SubjectRef: person.String(),
			DueAt:      time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("seeding a %s request: %v", kind, err)
		}
		got, err := store.GetDSR(e.Admin(), created.ID)
		if err != nil {
			t.Fatalf("reading back the %s request: %v", kind, err)
		}
		if got.Kind == "access" {
			t.Fatalf("a %s request reads back as access, so the handler's kind check cannot refuse it", kind)
		}
	}
}

func TestAnAccessRequestNamingNobodyHasNothingToAssemble(t *testing.T) {
	// subject_ref is free text until somebody resolves it to a person. The
	// erasure path already refuses one that names nobody; the access path owes
	// the same answer rather than a confusing one from further in.
	e := Setup(t)
	store := consent.NewStore(e.DB())
	created, err := store.CreateDSR(e.Admin(), consent.CreateDSRInput{
		Kind:       "access",
		SubjectRef: subjectAddress,
		DueAt:      time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seeding an access request naming an address: %v", err)
	}
	got, err := store.GetDSR(e.Admin(), created.ID)
	if err != nil {
		t.Fatalf("reading back the request: %v", err)
	}
	if _, parseErr := ids.Parse(got.SubjectRef); parseErr == nil {
		t.Fatal("an address parsed as a person id, so the handler's refusal is unreachable")
	}
}

func TestTheAssembledPackageIsSerializable(t *testing.T) {
	// The seam hands bytes across, so a package that cannot be marshalled is a
	// 500 on the one endpoint an officer needs under a statutory deadline.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	body, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("the assembled package does not serialize: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(body, &round); err != nil {
		t.Fatalf("the serialized package does not parse back: %v", err)
	}
	for _, section := range []string{"subject", "emails", "activities"} {
		if _, ok := round[section]; !ok {
			t.Errorf("the serialized package has no %q section", section)
		}
	}
}

// recordingAssembler stands in for the Art. 15 export so a handler test can
// assert whether it was reached at all.
//
// Whether it RAN is the assertion that matters: the route's whole authority
// argument is that the request is read through the queue's admin-only gate
// BEFORE anything is assembled. A test that only checked the status code would
// stay green if the two calls were swapped, and ops would have a route to any
// subject's entire record.
type recordingAssembler struct {
	calls int
}

func (a *recordingAssembler) AssemblePackage(context.Context, ids.UUID) ([]byte, error) {
	a.calls++
	return []byte(`{"subject":{}}`), nil
}

// downloadPackage drives the real handler, with the real store behind it.
func downloadPackage(
	ctx context.Context, t *testing.T, e *Env, requestID ids.UUID, assembler consent.SubjectAccessAssembler,
) *httptest.ResponseRecorder {
	t.Helper()
	h := consent.NewHandlers(e.DB()).WithSubjectAccessAssembler(assembler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/data-subject-requests/x/package", nil).WithContext(ctx)
	h.DownloadDataSubjectPackage(rec, req, crmcontracts.Id(requestID))
	return rec
}

func TestTheHandlerReadsTheRequestBeforeItAssemblesAnything(t *testing.T) {
	// The ordering the whole design rests on. AssembleSAR admits ops and
	// management; the queue's gate does not. Reading the request first is what
	// makes the route admin-only, and swapping the two calls would leave every
	// other test in this file green.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)
	request := seedAccessRequest(t, e, person.String())

	assembler := &recordingAssembler{}
	opsCtx := e.As(ids.NewV7(), []ids.UUID{e.Team1}, OpsPerms)
	rec := downloadPackage(opsCtx, t, e, request, assembler)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ops got %d from the package route, want 403", rec.Code)
	}
	if assembler.calls != 0 {
		t.Errorf("the assembler ran %d times for a caller the queue refuses: the request must be "+
			"read through the admin-only gate BEFORE anything is assembled", assembler.calls)
	}
}

func TestTheHandlerAnswersAnAdminWithThePackage(t *testing.T) {
	// The admit case. Without it the refusals above could all pass against an
	// authority that turns everybody away.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)
	request := seedAccessRequest(t, e, person.String())

	assembler := &recordingAssembler{}
	rec := downloadPackage(e.Admin(), t, e, request, assembler)

	if rec.Code != http.StatusOK {
		t.Fatalf("an admin got %d from the package route, want 200: %s", rec.Code, rec.Body.String())
	}
	if assembler.calls != 1 {
		t.Errorf("the assembler ran %d times for an admin, want 1", assembler.calls)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, request.String()) {
		t.Errorf("Content-Disposition = %q, want the request id in the filename so the copy an "+
			"officer sent ties to the row that recorded it", got)
	}
}

func TestTheHandlerRefusesARequestOfTheWrongKind(t *testing.T) {
	// An erasure is answered by its own path. Handing back a subject's whole
	// record to close a request that asked for something else is the export
	// nobody asked for.
	e := Setup(t)
	person := seedSubjectWithMail(t, e)
	store := consent.NewStore(e.DB())
	created, err := store.CreateDSR(e.Admin(), consent.CreateDSRInput{
		Kind: "erasure", SubjectRef: person.String(),
		DueAt: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seeding an erasure request: %v", err)
	}

	assembler := &recordingAssembler{}
	rec := downloadPackage(e.Admin(), t, e, created.ID, assembler)

	if rec.Code == http.StatusOK {
		t.Errorf("an erasure request produced a package: %s", rec.Body.String())
	}
	if assembler.calls != 0 {
		t.Errorf("the assembler ran for an erasure request, so the kind check does not guard it")
	}
}

func TestTheHandlerRefusesARequestNamingNobody(t *testing.T) {
	// subject_ref is free text until somebody resolves it to a person id.
	e := Setup(t)
	request := seedAccessRequest(t, e, subjectAddress)

	assembler := &recordingAssembler{}
	rec := downloadPackage(e.Admin(), t, e, request, assembler)

	if rec.Code == http.StatusOK {
		t.Errorf("a request naming an address produced a package: %s", rec.Body.String())
	}
	if assembler.calls != 0 {
		t.Errorf("the assembler ran for a request naming nobody, so the resolution check does not guard it")
	}
}

// seedAccessRequest lands one Art. 15 request naming whatever the caller says.
func seedAccessRequest(t *testing.T, e *Env, subjectRef string) ids.UUID {
	t.Helper()
	created, err := consent.NewStore(e.DB()).CreateDSR(e.Admin(), consent.CreateDSRInput{
		Kind: "access", SubjectRef: subjectRef,
		DueAt: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seeding an access request: %v", err)
	}
	return created.ID
}

// subjectAddress is the one correspondent these tests are about. Named because
// TestAnAccessRequestNamingNobodyHasNothingToAssemble asserts against the same
// string, and the two agreeing is what makes that test about the address rather
// than about a typo.
const subjectAddress = "betroffene@example.test"

// seedSubjectWithMail lands a person with an address and one message on their
// timeline, which is the least a package has to be able to hand back.
//
// The person goes through the store that writes one in production, so what the
// export reads back is the row a real creation makes rather than a shape only
// this test produces.
func seedSubjectWithMail(t *testing.T, e *Env) ids.UUID {
	const address = subjectAddress
	const subject = "Angebot vom Dienstag"
	t.Helper()
	person, err := e.People.CreatePerson(e.Admin(), people.CreatePersonInput{
		FullName: "Die Betroffene",
		Source:   "manual",
		Emails:   []people.PersonEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the subject: %v", err)
	}
	activity := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source, captured_by,
			                      counterparty_email, audience)
			VALUES ($1, 'email', $2, 'der Nachrichtentext', 'inbound', 'gmail',
			        'connector:gmail', $3, 'workspace')`, activity, subject, address); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`INSERT INTO activity_link (activity_id, entity_type, person_id)
			 VALUES ($1, 'person', $2)`, activity, person.Id)
		return err
	}); err != nil {
		t.Fatalf("seeding the subject's correspondence: %v", err)
	}
	return ids.UUID(person.Id)
}
