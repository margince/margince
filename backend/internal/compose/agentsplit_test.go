// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The buffered response the split gate replays through. What matters here is
// the shape of what reaches the wire: a client of this surface parses RFC 7807
// on every refusal, so one path out of the handler answering something else is
// a client that cannot read its own error.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// bufferedFor builds a buffered response already holding a status and headers,
// as it would after the wrapped handler answered.
func bufferedFor(status string) *bufferedResponse {
	b := newBufferedResponse()
	b.header.Set("Content-Type", "application/json")
	b.header.Set("X-Recorded", status)
	return b
}

func TestFlushJSONReplacesTheBodyAndItsContentLength(t *testing.T) {
	b := bufferedFor("ok")
	b.WriteHeader(http.StatusOK)
	if _, err := b.Write([]byte(`{"id":"1"}`)); err != nil {
		t.Fatalf("buffering the original body: %v", err)
	}
	rec := httptest.NewRecorder()

	b.flushJSON(rec, httptest.NewRequest(http.MethodPatch, "/v1/deals/1", nil),
		map[string]any{"id": "1", "staged_approval": map[string]any{"approval_id": "a1"}})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want the buffered 200", rec.Code)
	}
	if got := rec.Header().Get("X-Recorded"); got != "ok" {
		t.Errorf("the buffered headers were dropped: X-Recorded = %q", got)
	}
	body := rec.Body.String()
	// The Content-Length of the ORIGINAL body no longer applies, and a stale
	// one truncates the record for every client that honours it.
	if got, want := rec.Header().Get("Content-Length"), len(body); got != "" && got != itoa(want) {
		t.Errorf("Content-Length = %q, want %d — the length of what was actually written", got, want)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("the replayed body is not JSON: %v (%s)", err, body)
	}
	if _, staged := decoded["staged_approval"]; !staged {
		t.Errorf("the staging note was not spliced into the replayed record: %s", body)
	}
}

// unmarshalable is a payload value encoding/json refuses. A map decoded from
// JSON cannot hold one in production — which is exactly why the branch needs a
// test: it is unreachable by the handler and would otherwise never run.
type unmarshalable struct{}

func (unmarshalable) MarshalJSON() ([]byte, error) { return nil, errNotEncodable }

var errNotEncodable = &json.UnsupportedValueError{Str: "deliberately unencodable"}

// TestFlushJSONAnswersAMarshalFailureAsAProblemDocument — the staging already
// exists at this point, so the client must be told the request failed rather
// than handed a truncated record. Through httperr like every other refusal on
// this surface: a text/plain body here is one a client parsing the contract's
// error shape cannot read, on the path it needs most.
func TestFlushJSONAnswersAMarshalFailureAsAProblemDocument(t *testing.T) {
	b := bufferedFor("ok")
	b.WriteHeader(http.StatusOK)
	rec := httptest.NewRecorder()

	b.flushJSON(rec, httptest.NewRequest(http.MethodPatch, "/v1/deals/1", nil),
		map[string]any{"boom": unmarshalable{}})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 — the staging exists and the record could not be built", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "problem+json") {
		t.Errorf("Content-Type = %q, want an RFC 7807 problem document", ct)
	}
	// The marshal error names Go internals; it belongs in the server log.
	if strings.Contains(rec.Body.String(), "unencodable") {
		t.Errorf("the marshal error leaked to the client: %s", rec.Body.String())
	}
}

// itoa keeps the length comparison above readable without pulling strconv into
// a file that needs it once.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// allHumanOwned answers the ownership probe by naming every field the patch
// touches — the shape that leaves SplitHumanOwned's AutoExecute half empty,
// so splitHumanOwnedUpdate's terminal branch (agentsplit.go: "every touched
// field is human-owned") is the one under test rather than the mixed one.
type allHumanOwned struct{}

func (allHumanOwned) HumanOwnedConflicts(_ context.Context, _ string, _ ids.UUID, patch json.RawMessage) ([]string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(patch, &fields); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names, nil
}

// The split's all-human-owned branch resolves through the SAME command seam
// every other registered patch does (agentsplit.go's own comment on this
// branch states why), so it owes the record the same refusal
// refuseStagingElsewhere gives every other stager: an approval against a
// target whose authority lives elsewhere could never be redeemed, since
// redemption's version pin reads our own tables. Before that registration
// this branch ran no such check at all — an agent patching a mirrored deal
// with every field human-owned got a staged approval instead.
func TestSplitAllHumanOwnedRefusesAnExternallyHeldRecord(t *testing.T) {
	staging := &capturingApprovals{}
	pol := agentPolicy{Op: "updatePerson", Access: accessTool, Tool: "update_record", RecordType: recordTypePerson}
	personID := ids.NewV7()
	body := []byte(`{"full_name":"Overwritten"}`)

	req := patchRequest("/v1/people", personID, body)
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("the handler ran — every field was human-owned, so nothing should have auto-executed")
	})

	splitHumanOwnedUpdate(rec, req, next,
		splitUpdateDeps{staging: staging, commands: restCommandDeps{records: mirroredRecord{}}, ownership: allHumanOwned{}},
		pol, body)

	if staging.last.Tool != "" {
		t.Errorf("an approval was staged for %q against a record whose authority lives elsewhere — "+
			"nobody could ever release it", staging.last.Tool)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an externally-held target answered %d, want %d (unsupported_by_sor) — the refusal a "+
			"caller gets must name why the patch cannot be governed here", rec.Code, http.StatusUnprocessableEntity)
	}
}

// mixedHumanOwned answers the ownership probe by naming only ONE field as
// human-owned, leaving any other touched field to auto-execute — the shape
// that sends splitHumanOwnedUpdate down applyAutoExecuteAndStageResidue's
// residue path (split.AutoExecute != nil) rather than allHumanOwned's
// all-refused terminal branch.
type mixedHumanOwned struct{ conflict string }

func (m mixedHumanOwned) HumanOwnedConflicts(context.Context, string, ids.UUID, json.RawMessage) ([]string, error) {
	return []string{m.conflict}, nil
}

// A refusal describes a request that changed nothing
// (margince/margince#1073). The residue path's own refusals — the
// resolver's Guards among them — are settled BEFORE the auto-execute half is
// dispatched, so a target this door will not stage against costs the caller a
// retry rather than leaving half a patch committed under a 4xx that says the
// change was refused.
//
// Driven with the mirrored record the all-human-owned branch uses
// (TestSplitAllHumanOwnedRefusesAnExternallyHeldRecord) and the MIXED ownership
// that reaches the residue path: before the ordering fix the handler ran first,
// so this same call answered 422 with the agent-owned half already written.
func TestTheResiduePathRefusesAnExternallyHeldRecordBeforeAnythingIsWritten(t *testing.T) {
	orgID := ids.NewV7()
	staging := &capturingApprovals{}
	pol := agentPolicy{Op: "updateOrganization", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization}
	body := []byte(`{"display_name":"Renamed GmbH","industry":"software"}`)
	req := patchRequest("/v1/organizations", orgID, body)
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran — the agent-owned half was written for a call this door then refused, " +
			"which is the partial write a refusal must never describe")
	})

	admitAgentCall(rec, req, next, admissionOutcome{
		staging: staging, ownership: mixedHumanOwned{conflict: "display_name"},
		commands: restCommandDeps{records: mirroredRecord{}}, pol: pol, body: body,
		registry: agents.NewRegistry(nil, auth.NewGate(fullSeat{})),
	})

	if staging.last.Tool != "" {
		t.Errorf("an approval was staged for %q against a record whose authority lives elsewhere — "+
			"nobody could ever release it", staging.last.Tool)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an externally-held target answered %d, want %d (unsupported_by_sor)", rec.Code,
			http.StatusUnprocessableEntity)
	}
}

// A residue staged under an Idempotency-Key must be redeemable by the retry
// the staging note actually instructs.
//
// This is the whole of the defect. The auto-execute half of a mixed patch
// answers 2xx under the caller's key, and only a 2xx settles an idempotency
// claim — so that key is spent, permanently. The retry the note asks for
// ("repeat this request with ONLY those fields and the X-Approval-Token
// header") therefore cannot present it: re-using it never even reaches the
// gate, because the idempotency middleware sits outside it and answers the
// residue body as a digest mismatch. Hashing the key into the residue's
// identity made the only retry that CAN arrive unable to match, so the human's
// approval was void and the withheld field could never be written.
//
// Asserted as the redemption side computes it, not as a property of the
// canonicalization alone: redeemIfPresented hashes the retry with
// keyBindsTheRetry — it cannot know which kind of approval it is about to
// redeem — so what has to agree is the STAGED hash and the hash of a keyless
// retry carrying the same residue.
func TestAResidueStagedUnderAnIdempotencyKeyIsRedeemableByItsRetry(t *testing.T) {
	orgID := ids.NewV7()
	staging := &capturingApprovals{}
	pol := agentPolicy{Op: "updateOrganization", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization}
	body := []byte(`{"display_name":"Renamed GmbH","industry":"software"}`)
	req := operandRequest(http.MethodPatch, "/v1/organizations", orgID.String(), "", "", body)
	req.Header.Set(idempotencyKeyHeader, "01J0-agent-retry-key")
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"` + orgID.String() + `","display_name":"Renamed GmbH","version":3}`))
	})

	admitAgentCall(httptest.NewRecorder(), req, next, admissionOutcome{
		staging: staging, ownership: mixedHumanOwned{conflict: "display_name"},
		commands: restCommandDeps{records: seamRecord{}}, pol: pol, body: body,
		registry: agents.NewRegistry(nil, auth.NewGate(fullSeat{})),
	})
	if staging.last.DiffHash == "" {
		t.Fatal("nothing was staged, so there is no residue identity to redeem")
	}

	// The retry the staging note instructs: the withheld fields alone, the
	// approval token, and NO idempotency key — the original is settled and a
	// fresh one would be a different call.
	retry := operandRequest(http.MethodPatch, "/v1/organizations", orgID.String(), "", "",
		[]byte(staging.last.ProposedChange))
	_, retryHash, err := canonicalRESTCall(pol.Op, retry.URL.Path, retry.Header,
		[]byte(`{"display_name":"Renamed GmbH"}`), keyBindsTheRetry)
	if err != nil {
		t.Fatalf("canonicalizing the retry answered %v", err)
	}

	if retryHash != staging.last.DiffHash {
		t.Errorf("the approved retry hashes to %s but the residue was staged as %s — the human's approval "+
			"is unredeemable and the withheld field can never be written",
			retryHash, staging.last.DiffHash)
	}
}

// ownedOnlyFor answers the ownership probe for exactly ONE record id and
// nothing else, which is what makes the test below able to fail.
//
// allHumanOwned above ignores the id entirely, so a probe asked about the WRONG
// record would still report a conflict and the test would pass while the gate
// was broken. The defect this guards against is precisely an id mismatch, so
// the double has to be the thing that can tell two ids apart.
type ownedOnlyFor struct{ id ids.UUID }

func (o ownedOnlyFor) HumanOwnedConflicts(_ context.Context, _ string, target ids.UUID, patch json.RawMessage) ([]string, error) {
	if target != o.id {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(patch, &fields); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names, nil
}

// A sub-resource patch asks the ownership probe about the record it WRITES.
//
// On a route like /parents/{id}/items/{itemId} the route's own {id} is the
// PARENT, so a probe reading {id} asks "who typed this field on item
// ⟨parent-id⟩" — a question no audit row answers. It misses, the split sees no
// conflict, and an agent overwrite of a human-typed field auto-executes instead
// of staging. The §2.1 protection would be off while the route still looked
// governed.
//
// No agent-reachable route is shaped this way today: the Deal Room document
// patch that was is human-only now. The mechanism (patchTargetParam) stays for
// the next one, and so does this test — rediscovering the trap is the expensive
// half, and an untested mechanism is one somebody deletes as unused.
func TestASubResourcePatchProbesTheRecordItWrites(t *testing.T) {
	// Registered for this test only: no live route is shaped this way, and the
	// mechanism is what is under test rather than any one operation.
	const opPatchSubResource = "patchSubResourceUnderTest"
	patchTargetParam[opPatchSubResource] = "documentId"
	restCommands[opPatchSubResource] = roomItemPatch("documentId")
	t.Cleanup(func() {
		delete(patchTargetParam, opPatchSubResource)
		delete(restCommands, opPatchSubResource)
	})

	roomID, documentID := ids.NewV7(), ids.NewV7()
	staging := &capturingApprovals{}
	pol := agentPolicy{
		Op: opPatchSubResource, Access: accessTool,
		Tool: "update_record", RecordType: recordTypeDealRoomDocument,
	}
	body := []byte(`{"title":"wording a human typed"}`)
	req := operandRequest(http.MethodPatch, "/v1/deal-rooms", roomID.String(), "documentId", documentID.String(), body)
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("the handler ran — the title is human-owned, so the write must stage, not apply")
	})

	admitAgentCall(rec, req, next, admissionOutcome{
		staging: staging, ownership: ownedOnlyFor{id: documentID},
		commands: restCommandDeps{records: seamRecord{}}, pol: pol, body: body,
	})

	if staging.last.TargetID != documentID {
		t.Fatalf("staged target id = %s, want the task %s — an approval binding to the room names a "+
			"different record than the one the released call goes on to write", staging.last.TargetID, documentID)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d (approval_required) — an agent silently overwriting a human-typed "+
			"to-do is the §2.1 protection this test exists to hold", rec.Code, http.StatusForbidden)
	}
}

// failingApprovals is the staging seam refusing after the auto-execute half has
// already committed — the race the split cannot design away, because approvals
// resolves its own target version inside the staging transaction and that
// transaction can only run after the write it is staged against.
type failingApprovals struct{ capturingApprovals }

var errStagingRefused = errors.New("the record's authority moved between the two writes")

func (failingApprovals) StageCall(context.Context, agents.StageRequest) (ids.ApprovalID, bool, error) {
	return ids.ApprovalID{}, false, errStagingRefused
}

// A refusal that follows a committed write says so, so the key is not given
// back.
//
// The split writes the agent-owned fields and then stages the human-owned
// residue. A staging failure answers a refusal for a request that already
// wrote, and the idempotency middleware reads every non-2xx the same way: it
// releases the claim. The retry the caller is entitled to make then runs the
// applied half a second time under the same key, which is the one thing the key
// exists to prevent.
//
// So the handler reports the write, and this asserts the report — the middleware
// is a layer up and settleClaim's own case covers what it does with it.
func TestARefusalAfterTheAppliedHalfReportsThatItWrote(t *testing.T) {
	orgID := ids.NewV7()
	pol := agentPolicy{Op: "updateOrganization", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization}
	body := []byte(`{"display_name":"Renamed GmbH","industry":"software"}`)
	req := patchRequest("/v1/organizations", orgID, body)
	ctx, effect := withWriteEffect(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	// The agent-owned half commits, exactly as the real handler does.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"id":"` + orgID.String() + `","version":2}`)); err != nil {
			t.Fatalf("writing the applied half: %v", err)
		}
	})

	admitAgentCall(rec, req, next, admissionOutcome{
		staging: &failingApprovals{}, ownership: mixedHumanOwned{conflict: "display_name"},
		commands: restCommandDeps{records: seamRecord{}}, pol: pol, body: body,
		registry: agents.NewRegistry(nil, auth.NewGate(fullSeat{})),
	})

	if rec.Code >= 200 && rec.Code <= 299 {
		t.Fatalf("a failed staging answered %d — the caller is told the whole patch landed", rec.Code)
	}
	if !effect.committed {
		t.Error("the refusal did not report that it had written, so the idempotency layer gives the key back " +
			"and the retry runs the applied half a second time under it")
	}
	// And the sentence still names what did land, which is the half a caller
	// cannot discover any other way.
	if !strings.Contains(rec.Body.String(), "display_name") {
		t.Errorf("the refusal reads %q and does not name the withheld fields", rec.Body.String())
	}
}

// The control, one outcome apart. Without it the case above would pass against
// a door that reported a write on every path, which would strand the key of
// every ordinary refusal — a caller unable to retry a call that changed nothing.
func TestARefusalBeforeAnythingIsWrittenReportsNoWrite(t *testing.T) {
	orgID := ids.NewV7()
	pol := agentPolicy{Op: "updateOrganization", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization}
	body := []byte(`{"display_name":"Renamed GmbH","industry":"software"}`)
	req := patchRequest("/v1/organizations", orgID, body)
	ctx, effect := withWriteEffect(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the handler ran for a call this door refuses before the write")
	})

	// An externally-held record: refused by the target resolver, ahead of the
	// auto-execute half.
	admitAgentCall(rec, req, next, admissionOutcome{
		staging: &capturingApprovals{}, ownership: mixedHumanOwned{conflict: "display_name"},
		commands: restCommandDeps{records: mirroredRecord{}}, pol: pol, body: body,
		registry: agents.NewRegistry(nil, auth.NewGate(fullSeat{})),
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if effect.committed {
		t.Error("a refusal that wrote nothing reported a write, which strands the caller's idempotency key " +
			"on a call they are entitled to retry unchanged")
	}
}

// Every route the field split governs settles its refusals BEFORE the
// auto-execute half runs — not only the one operation this was first proven for.
//
// Registering all the whole-record patch routes put patchResolver.Guards on
// this path: a records.Read plus the external-system-of-record refusal that the
// split path never ran on its own before. Deliberate, and until now exercised
// for updateOrganization alone, with the policy written as a literal beside the
// assertion — so the rest carried the ordering on the strength of sharing a
// code path, which is the argument that stops being true the moment one of them
// stops sharing it.
//
// The corpus is production's OWN routing condition (reachesTheHumanOwnedSplit,
// agentgateauto.go) over the generated policy table, and the policies are its
// rows rather than literals. A route that joins this family is walked here
// without anybody remembering to add it; one that leaves stops being walked at
// the same moment it stops being routed. Nothing narrows it further.
//
// What each case proves is the ORDERING, which is the whole of #1073: a target
// this door will not stage against must cost the caller a retry, never a
// half-applied patch under a 4xx saying the change was refused. The handler
// failing the test if it runs at all is that assertion — everything the door
// can refuse on is a function of the policy, the path and the staged sub-patch,
// all of which exist before dispatch.
//
// Every route lands on ONE of two positive assertions, and that is what keeps
// the walk from reading a shrinking corpus as success: a served record type
// must be refused before dispatch, and an unserved one — where Guards has no
// seam row and so no refusal to make — must reach the handler. Silence is not
// available to either.
func TestEveryWholeRecordPatchRunsItsGuardsBeforeTheAutoExecuteHalf(t *testing.T) {
	family := splitGovernedRoutes()
	served := 0
	for route, pol := range family {
		// patchResolver.Guards refuses on the SEAM's answer about the row —
		// unreadable, or held in another system of record — so a record type
		// the seam does not serve has no such answer and no refusal to make.
		//
		// The exclusion is datasource's own list of served types, which is
		// production data rather than a set written down here: a type that
		// joins the seam joins this assertion in the same commit, with nobody
		// to remember it. That is what stops this walk from quietly shrinking
		// — the one failure mode a census does not report.
		if !servedByTheRecordSeam(pol.RecordType) {
			continue
		}
		// And a route that sends no patchable field has no human-owned one
		// either, so the ownership probe finds no conflict and the call
		// dispatches without ever reaching the staging arm Guards sits on.
		// Read off the contract, the same place the body itself comes from.
		conflict, body := patchBodyFromTheContract(t, pol.Op)
		if body == nil {
			continue
		}
		served++
		t.Run(pol.Op, func(t *testing.T) {
			staging := &capturingApprovals{}
			rec := httptest.NewRecorder()
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Errorf("%s: the handler ran for a record whose authority lives elsewhere — the "+
					"agent-owned half was written for a call this door then refused", route)
			})

			admitAgentCall(rec, splitRequestFor(route, body), next, admissionOutcome{
				staging: staging, ownership: mixedHumanOwned{conflict: conflict},
				commands: restCommandDeps{records: mirroredRecord{}}, pol: pol, body: body,
				registry: agents.NewRegistry(nil, auth.NewGate(fullSeat{})),
			})

			if staging.last.Tool != "" {
				t.Errorf("%s: an approval was staged against a record whose authority lives elsewhere — "+
					"nobody could ever release it", route)
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s: an externally-held target answered %d, want %d (unsupported_by_sor)",
					route, rec.Code, http.StatusUnprocessableEntity)
			}
		})
	}
	if served == 0 {
		t.Fatal("no route the field split governs both targets a seam-served record type and patches a " +
			"field — this walk drove nothing, and would pass against a guard that had stopped running " +
			"on the path entirely")
	}
}

// splitGovernedRoutes are the agent-reachable routes the field split governs,
// asked of production's own condition rather than restated here.
func splitGovernedRoutes() map[string]agentPolicy {
	family := map[string]agentPolicy{}
	for route, pol := range agentPolicies {
		if pol.Access == accessTool && reachesTheHumanOwnedSplit(pol) {
			family[route] = pol
		}
	}
	return family
}

// splitRequestFor is the request the router would hand the gate for one route
// of this family, carrying the body the contract declares.
//
// Built from the route TEMPLATE, so every path parameter is bound — not just
// the routed {id}. Several of these operations name a second operand in the
// path (the fact key, the profile field, the stakeholder), and a request that
// left one unbound would be refused for the missing segment rather than by the
// guard under test: a 500 that reads exactly like the refusal being asserted.
func splitRequestFor(route string, body []byte) *http.Request {
	req := syntheticOperandRequest(route, ids.NewV7())
	if body == nil {
		return req
	}
	withBody := req.Clone(req.Context())
	withBody.Body = io.NopCloser(bytes.NewReader(body))
	withBody.ContentLength = int64(len(body))
	return withBody
}

// patchBodyFromTheContract builds one operation's minimal patch body out of
// crm.yaml, and names which of its fields the ownership probe will call
// human-owned.
//
// TWO fields where the schema has them, because the RESIDUE path is reached
// only by a mixed patch: one field conflicts and is staged, the rest
// auto-execute. A schema declaring one takes allHumanOwned's terminal branch
// instead, which refuses the same way and for the same reason — this walk is
// about the ordering both branches share, and neither may dispatch first.
//
// Read from the contract rather than written here because the split reasons
// about field NAMES: a name this schema does not carry would exercise the walk
// against a patch the route cannot receive, and the operation whose real fields
// stopped matching would be the one case that quietly went on passing.
func patchBodyFromTheContract(t *testing.T, op string) (conflict string, body []byte) {
	t.Helper()
	for _, item := range loadContract(t).Paths.Map() {
		for _, candidate := range item.Operations() {
			if candidate.OperationID != op || candidate.RequestBody == nil {
				continue
			}
			schema := candidate.RequestBody.Value.Content.Get("application/json").Schema.Value
			names := slices.Sorted(maps.Keys(schema.Properties))
			if len(names) == 0 {
				t.Fatalf("%s declares a request body with no patchable field, so the ownership probe "+
					"below has nothing to name and this walk sends an empty patch", op)
			}
			fields := map[string]any{}
			for _, name := range names[:min(2, len(names))] {
				fields[name] = placeholderFor(schema.Properties[name].Value)
			}
			encoded, err := json.Marshal(fields)
			if err != nil {
				t.Fatalf("%s: encoding its own fields: %v", op, err)
			}
			return names[0], encoded
		}
	}
	// No request body: the route sends no fields, so there is nothing for the
	// ownership probe to be asked about and nothing to split. The refusal under
	// test is Guards' own, which reads the ROW rather than the fields, so it is
	// owed here exactly as it is for a route that patches columns.
	return "", nil
}

// placeholderFor is a value of the type the contract declares for a field.
//
// The values are never read — every case here is decided before the patch
// reaches a writer — but they are typed anyway, because an untyped placeholder
// makes the body a shape the route does not accept, and the first case that DID
// read one would be proven against a request no caller can send.
//
//craft:ignore naked-any a JSON value of whichever type the schema declares — the return is fed straight to json.Marshal, and any narrower type would be one of these cases spelled as a union
func placeholderFor(schema *openapi3.Schema) any {
	switch {
	case len(schema.Enum) > 0:
		return schema.Enum[0]
	case schema.Type.Is("integer"), schema.Type.Is("number"):
		return 1
	case schema.Type.Is("boolean"):
		return true
	case schema.Type.Is("array"):
		return []any{}
	case schema.Type.Is("object"):
		return map[string]any{}
	default:
		return "placeholder"
	}
}

// servedByTheRecordSeam answers from datasource's own list, the same source
// agents.servedByTheRecordSeam reads — not a copy of the types it names today.
func servedByTheRecordSeam(rt agentRecordType) bool {
	return slices.Contains(datasource.EntityTypes(), datasource.EntityType(rt))
}
