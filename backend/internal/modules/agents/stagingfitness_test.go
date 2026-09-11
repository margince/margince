// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Derived coverage for the staging refusal (review-loop rule 2). Pinning the
// predicate alone cannot hold the invariant: refuseStagingElsewhere has to be
// CALLED by every tool that stages, and a per-site spec only ever covers the
// sites someone remembered. So this enumerates the core registry's
// StageInfo-shaped tools and requires each to refuse a target whose authority
// lives elsewhere — one with no args entry fails here, and one that forgets the
// guard fails here too.
//
// Two tools answer a DIFFERENT question here, and both are walked rather than
// excused:
//   - update_record stages twice over. Its per-field residue goes through
//     stageConflicts, which this walk cannot see —
//     TestUpdateRecordRefusesStagingForATargetHeldElsewhere is that path's pin.
//     Its StageInfo is the whole-call staging the contract's per-record-type
//     tier floor produces (#982), and that one IS walked here, because it reads
//     the record it patches exactly as its 🟡 siblings do.
//   - create_record stages a CREATE, which names no existing record, so there is
//     no target whose system of record could be elsewhere and nothing for
//     refuseStagingElsewhere to be called about. It is held to the invariant it
//     does have — stagesACreate below — rather than dropped from the count, so a
//     create that began inventing a target still fails something.
//
// One boundary these walks do NOT cover, stated so it does not read as covered:
//   - registerEveryStageableFamily names the registrars by hand. Every one that
//     installs a stageable tool is in it today, and the count assertion trips the
//     moment a named family grows or loses one — but a stageable tool installed
//     by a registrar nobody added there is invisible to the walk AND to the
//     count, because both are derived from what was registered. Nothing in Go
//     can enumerate the implementations of an unexported interface, so this is
//     the shape of miss that stays.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// elsewhereProvider serves every read as a record whose system of record is
// external — the shape overlay.Provider returns (Authoritative false).
type elsewhereProvider struct {
	datasource.SystemOfRecordProvider
}

func (elsewhereProvider) Read(_ context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	return datasource.Record{Ref: ref, Fields: json.RawMessage(`{"full_name":"Mirrored"}`)}, nil
}

// stageableToolArgs is the argument corpus every walk over the stageable set
// shares: one call per tool that reads an existing row, and the creates that
// read none. ONE table, because two walks with two tables is two answers to
// "which tools stage", and the count assertion each walk makes would then be
// pinning a different universe.
func stageableToolArgs() (reads, creates map[string]string) {
	contact, lead, deal, stage, activity := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	reads = map[string]string{
		"archive_record": fmt.Sprintf(`{"record_type":"contact","id":%q}`, contact),
		"promote_lead":   fmt.Sprintf(`{"lead_id":%q,"trigger":"meeting_booked"}`, lead),
		"merge_records":  fmt.Sprintf(`{"record_type":"contact","source_id":%q,"target_id":%q}`, ids.NewV7(), contact),
		"advance_deal":   fmt.Sprintf(`{"deal_id":%q,"to_stage_id":%q}`, deal, stage),
		"progress_deal":  fmt.Sprintf(`{"deal_id":%q,"to_stage_id":%q,"note":"n"}`, deal, stage),
		"send_message":   fmt.Sprintf(`{"activity_id":%q,"body":"b","consent_purpose":"support"}`, activity),
		"send_email":     fmt.Sprintf(`{"activity_id":%q,"to":["a@example.test"],"subject":"s","body":"b","consent_purpose":"support"}`, activity),
		// A booking anchors on no row, so its refusal has to come through a
		// LINK. Arguments with no links would read nothing and pass this walk
		// while proving nothing.
		"book_meeting": fmt.Sprintf(
			`{"start":"2026-08-03T09:00:00Z","end":"2026-08-03T09:30:00Z","subject":"s","links":[{"entity_type":"deal","entity_id":%q}]}`, deal),
		// The account-started send has no anchor either, and for the same
		// reason: it starts the conversation instead of answering one. Its
		// links are what carry the refusal here.
		"send_account_email": fmt.Sprintf(
			`{"to":["a@example.test"],"subject":"s","body":"b","consent_purpose":"support",`+
				`"links":[{"entity_type":"company","entity_id":%q}]}`, ids.NewV7()),
		// The whole-call staging the tier floor produces (#982). It patches an
		// existing row, so it carries the same obligation as its siblings.
		"update_record": fmt.Sprintf(`{"record_type":"contact","id":%q,"fields":{"full_name":"X"}}`, contact),
	}
	// A create names no existing record, so it has no target to probe. What it
	// owes instead is to stage the shape it claims: the record TYPE, and no id —
	// an id here would be a target the approvals surface probes for row scope and
	// pins a version against, neither of which exists yet.
	// The lifecycle, tag, import and enrich families, which the walk below
	// registers alongside the core and comms sets.
	company, tagA, tagB, importRun, project := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for name, in := range map[string]string{
		"relink_activity": fmt.Sprintf(
			`{"activity_id":%q,"entity_type":"deal","entity_id":%q}`, activity, deal),
		"relink_thread": fmt.Sprintf(
			`{"thread_key":"mail:%s","entity_type":"deal","entity_id":%q}`, activity, deal),
		"relink_activities": fmt.Sprintf(
			`{"activity_ids":[%q],"entity_type":"deal","entity_id":%q}`, activity, deal),
		"disqualify_lead":       fmt.Sprintf(`{"lead_id":%q}`, lead),
		"demote_lead":           fmt.Sprintf(`{"lead_id":%q,"reason":"too early"}`, lead),
		"advance_project_phase": fmt.Sprintf(`{"project_id":%q,"to_phase":"delivering"}`, project),
		"merge_tags":            fmt.Sprintf(`{"tag_id":%q,"into_tag_id":%q}`, tagA, tagB),
		"commit_import":         fmt.Sprintf(`{"run_id":%q}`, importRun),
		"enrich":                fmt.Sprintf(`{"company_id":%q}`, company),
	} {
		reads[name] = in
	}

	creates = map[string]string{
		"create_record": `{"record_type":"contact","fields":{"full_name":"Fresh"}}`,
	}
	return reads, creates
}

// registerEveryStageableFamily installs every registrar in this package that
// installs a stageable tool, so a walk over registry.tools sees the whole set.
//
// The seams are nil where StageInfo does not reach them: a staging read asks
// the PROVIDER what the target is, and the executor behind the verb is only
// called when the approval is redeemed. Where one is reached — tags, imports —
// a double is supplied.
//
// Registering every family is what the count assertion in each walk is worth.
// With two families registered the count could not change for a tool in a third,
// which is a census reporting PASS over two thirds of its subject.
func registerEveryStageableFamily(r *Registry, p datasource.SystemOfRecordProvider, comms Comms) {
	RegisterCoreTools(r, p, fixedStages{semantic: "won"}, nil, noConflicts{}, nil, nil)
	RegisterCommsTools(r, comms, p)
	RegisterLifecycleTools(r, p, nil, nil, nil, nil)
	RegisterTagTools(r, stagingTags{})
	RegisterImportTools(r, stagingImports{})
	RegisterEnrichTool(r, p, nil)
}

func TestEveryStageableToolRefusesATargetHeldElsewhere(t *testing.T) {
	args, stagesACreate := stageableToolArgs()

	registry := NewRegistry(&recordingApprovals{}, nil)
	// fixedStages only satisfies the constructor: refuseStagingElsewhere returns
	// before advance_deal/progress_deal reach StageSemantic, so its answer is
	// never read on this path.
	registerEveryStageableFamily(registry, elsewhereProvider{}, &recordingComms{})

	// registry.tools IS the universe — walking Specs() and looking the name back
	// up adds a miss branch that could silently hide a tool from this pin.
	walked := 0
	for name, tool := range registry.tools {
		stageable, isStageable := tool.(stageableTool)
		if !isStageable {
			continue
		}
		walked++
		if in, creates := stagesACreate[name]; creates {
			info, err := stageable.StageInfo(context.Background(), json.RawMessage(in))
			if err != nil {
				t.Errorf("%s.StageInfo err = %v, want a staged create — a create reads no record, "+
					"so nothing here should refuse it", name, err)
			}
			if !info.TargetID.IsZero() {
				t.Errorf("%s staged target id %s; a create has no row yet, and naming one makes the "+
					"approvals surface probe and pin a record that does not exist", name, info.TargetID)
			}
			continue
		}
		if reason, own := stagesOverOurOwnTables[name]; own {
			// Named rather than skipped, and named with the reason: a tool that
			// simply vanished from this walk would be indistinguishable from one
			// that forgot the guard.
			info, err := stageable.StageInfo(context.Background(), json.RawMessage(args[name]))
			if err != nil {
				t.Errorf("%s.StageInfo err = %v; it stages over %s, so nothing here should refuse it",
					name, err, reason)
			}
			if info.Summary == "" {
				t.Errorf("%s staged nothing describable", name)
			}
			continue
		}
		in, known := args[name]
		if !known {
			t.Errorf("%s can stage an approval but this pin carries no arguments for it — "+
				"add them, so its refusal of an externally-held target is actually exercised", name)
			continue
		}
		if _, err := stageable.StageInfo(context.Background(), json.RawMessage(in)); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
			t.Errorf("%s.StageInfo err = %v, want ErrUnsupportedBySoR — it would mint an approval "+
				"no human can release, because redemption re-reads a row this record does not have", name, err)
		}
	}
	if pinned := len(args) + len(stagesACreate); walked != pinned {
		t.Errorf("walked %d stageable core tools, pinned %d — the core set changed, so a staging site "+
			"may now be unexercised", walked, pinned)
	}
}

// A merge touches TWO records, so validating only the pinned survivor leaves
// the other half unguarded: the merge archives and relinks the source, and an
// externally-held source under a locally-authoritative survivor is still a
// change no approval could release.
func TestMergeRefusesAnExternallyHeldSourceUnderALocalSurvivor(t *testing.T) {
	survivor, src := ids.NewV7(), ids.NewV7()
	survivorRef := datasource.EntityRef{Type: datasource.EntityContact, ID: survivor}
	sourceRef := datasource.EntityRef{Type: datasource.EntityContact, ID: src}
	p := &fakeSoR{records: map[datasource.EntityRef]datasource.Record{
		survivorRef: nativeRecord(datasource.Record{Ref: survivorRef, Fields: json.RawMessage(`{}`), Version: 4}),
		// Deliberately unstamped: this record's authority lives elsewhere.
		sourceRef: {Ref: sourceRef, Fields: json.RawMessage(`{}`)},
	}}

	_, err := mergeRecords{p: p}.StageInfo(context.Background(),
		json.RawMessage(fmt.Sprintf(`{"record_type":"contact","source_id":%q,"target_id":%q}`, src, survivor)))

	if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("StageInfo err = %v, want ErrUnsupportedBySoR — the merge source was not validated", err)
	}
}

// localProvider serves every read as a record this installation owns, so a
// staging walk reaches the summary instead of being refused at the door.
type localProvider struct {
	datasource.SystemOfRecordProvider
}

// A local provider archives, which the staging seam asks BEFORE it stages: a
// provider that cannot is refused at the door and never reaches a summary.
func (localProvider) ArchivableTypes(context.Context) ([]datasource.EntityType, error) {
	return []datasource.EntityType{datasource.EntityContact}, nil
}

func (localProvider) RefuseArchive(context.Context, datasource.EntityRef) error { return nil }

func (localProvider) ArchiveAt(_ context.Context, in datasource.ArchiveInput) (datasource.EntityRef, error) {
	return in.Ref, nil
}

func (localProvider) Read(_ context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	return nativeRecord(datasource.Record{
		Ref: ref, Version: 1,
		// One record shape serving every tool in the corpus: a name for the
		// labels a summary is written from, and the channel kind the reply
		// tools ask for before they will describe a send.
		Fields: json.RawMessage(`{"full_name":"Ada Lovelace","name":"Ada Lovelace",` +
			`"kind":"message","channel_provider":"telegram"}`),
	}), nil
}

// A staged call has to say WHAT it staged, to the caller that wrote it.
//
// The summary is composed for the human's inbox card, and until it also reached
// the caller an agent could relay only that something was pending: it told the
// user "a change will be applied once approved" and could not say which change,
// because the sentence describing it lived on a card the agent never reads. An
// empty summary is the same failure one tool at a time, so it is asked of every
// stageable tool rather than of the two that happened to be looked at.
//
// Walked over the SAME corpus as the refusal pin above, so a tool cannot be
// stageable here and absent there.
func TestEveryStageableToolSaysWhatItWouldDo(t *testing.T) {
	args, stagesACreate := stageableToolArgs()

	registry := NewRegistry(&recordingApprovals{}, nil)
	registerEveryStageableFamily(registry, localProvider{}, &recordingComms{})

	walked := 0
	for name, tool := range registry.tools {
		stageable, isStageable := tool.(stageableTool)
		if !isStageable {
			continue
		}
		walked++
		in, known := args[name]
		if !known {
			in, known = stagesACreate[name]
		}
		if !known {
			t.Errorf("%s can stage an approval and this walk carries no arguments for it", name)
			continue
		}
		info, err := stageable.StageInfo(context.Background(), json.RawMessage(in))
		if err != nil {
			// A tool this corpus cannot drive to a summary is reported rather
			// than skipped: a skip here is how a census stops asking.
			t.Errorf("%s.StageInfo err = %v against a locally-held record — the walk cannot "+
				"reach its summary, so nothing holds it", name, err)
			continue
		}
		if info.Summary == "" {
			t.Errorf("%s stages with no summary, so the human's card describes nothing and the "+
				"caller can only report that something is pending", name)
		}
	}
	if pinned := len(args) + len(stagesACreate); walked != pinned {
		t.Errorf("walked %d stageable core tools, pinned %d", walked, pinned)
	}
}

// And the one exit every stageable tool leaves through carries that summary
// into the answer. Asserted at stageRefusedCall rather than per tool: it is the
// single place the card's sentence and the caller's refusal are joined, so a
// tool cannot be correct here and wrong on its own path.
func TestTheStagedRefusalCarriesTheSummaryTheCardWasGiven(t *testing.T) {
	approvals := &recordingApprovals{}
	registry := NewRegistry(approvals, nil)
	registerEveryStageableFamily(registry, localProvider{}, &recordingComms{})

	args, _ := stageableToolArgs()
	in := json.RawMessage(args["archive_record"])
	tool := registry.tools["archive_record"]

	err := registry.stageRefusedCall(context.Background(), tool, "archive_record", in, "hash",
		apperrors.ErrRequiresApproval)

	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("staging answered %v, want a StagedApprovalError", err)
	}
	if len(approvals.staged) != 1 {
		t.Fatalf("staged %d calls, want 1", len(approvals.staged))
	}
	card := approvals.staged[0].Summary
	if card == "" {
		t.Fatal("the card was given no summary, so this pair compares two empty strings")
	}
	if staged.Summary != card {
		t.Errorf("the human's card says %q and the caller is told %q — a contact and an agent "+
			"waiting on two descriptions of one staged change", card, staged.Summary)
	}
	if !strings.Contains(err.Error(), card) {
		t.Errorf("the answer does not repeat the summary:\n%s", err.Error())
	}
}

// gatekit:fixture the stageable tools whose staged target is a row this
// installation always owns, each with what that row is — a statement about the
// tool, not a cost being excused
//
// stagesOverOurOwnTables names the stageable tools whose target is not a
// provider record at all, with the reason — so refuseStagingElsewhere has
// nothing to be called about for them. They are still walked, for the summary
// every staging owes, rather than dropped from the count where a tool that lost
// its guard could hide.
var stagesOverOurOwnTables = map[string]string{
	"merge_tags": "the vocabulary table this installation always owns, read through the tag " +
		"seam rather than the system-of-record provider",
	"commit_import": "an import RUN, a row in this installation's own tables that no external " +
		"system of record ever holds",
	"relink_thread": "a THREAD KEY, which names captured mail this installation holds and never a " +
		"row in another system of record — commandrelinkbatch refuses a thread key destination " +
		"that needs a human for the same reason",
}

// stagingTags answers the two reads mergeTags makes before it describes a
// merge: a staging summary names both words, so both have to resolve.
type stagingTags struct{ Tags }

func (stagingTags) GetTag(_ context.Context, tagID ids.UUID) (TagDetail, error) {
	return TagDetail{Tag: Tag{TagID: tagID, Name: "Strategic Account"}}, nil
}

// RecordTagTypes is read at registration, not at staging: the tag registrar
// asks it while building the apply/remove schema.
func (stagingTags) RecordTagTypes() []string { return []string{"contact", "company", "deal"} }

// TaggableTypes is read at registration too, by apply_tag's schema.
func (stagingTags) TaggableTypes() []string { return []string{"contact", "company", "deal"} }

// stagingImports serves the run commit_import reads before it can describe what
// committing would do.
type stagingImports struct{ Imports }

func (stagingImports) ReadRun(_ context.Context, id ids.UUID) (crmcontracts.ImportRun, error) {
	return crmcontracts.ImportRun{
		Id: openapi_types.UUID(id), Object: "contact", Status: "awaiting_approval",
	}, nil
}

func (stagingImports) ReadReport(context.Context, ids.UUID) (crmcontracts.ImportRunReport, error) {
	return crmcontracts.ImportRunReport{
		Disposition: crmcontracts.ImportRunDisposition{Created: 3, Updated: 1},
	}, nil
}
