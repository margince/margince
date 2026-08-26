// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Which record type does a call name, and can this verb serve it.
//
// Every implementation of recordTypedTool (tierfloor.go) lives here rather than
// beside its tool, because the two methods are one question asked of the whole
// tool set and the answers only make sense read together: what a floor can reach
// is exactly this file, and a verb missing from it is a verb an installation
// cannot tighten. Scattered across nine files that was invisible for nine verbs
// at once — the gap TestEveryStageableVerbCanBeFlooredBack now closes.
//
// Two shapes appear.
//
// A GENERIC verb takes a `record_type` argument and reads it: create_record,
// update_record, archive_record, merge_records. Its ServesRecordType asks the
// vocabulary that verb's own write path can express, so a floor declared for a
// type it cannot serve stays inert rather than staging an approval a human would
// spend on a call that dies at the provider.
//
// A SINGLE-TYPE verb takes no such argument, because the contract declares one
// record type for it and it performs that effect and no other. It answers a
// constant. Not an embedded shared type: every one of these tools is built as a
// keyed composite literal, so an embedded field would be left at its zero value
// at some construction site and answer "" — silently removing that door from the
// floor rather than failing to compile.
//
// Five of the single-type verbs — advance_deal, progress_deal, and the three
// relink verbs — resolve their tier DYNAMICALLY. A floor is not the same lever and does not
// duplicate the resolver: the resolver raises one call on that call's own facts,
// where a floor tightens every call of the verb for a record type. An
// installation that wants the whole verb confirmed cannot get there by waiting
// for the resolver to agree.

import (
	"encoding/json"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// --- generic verbs: the record type comes from the arguments ---

func (createRecord) RecordTypeOf(args json.RawMessage) string { return recordTypeArg(args) }

// The contract's own create bodies, which is the same vocabulary the input
// schema advertises.
func (createRecord) ServesRecordType(recordType string) bool {
	_, served := createShapes[datasource.EntityType(recordType)]
	return served
}

func (updateRecord) RecordTypeOf(args json.RawMessage) string { return recordTypeArg(args) }

// The contract's own update bodies.
func (updateRecord) ServesRecordType(recordType string) bool {
	_, served := updateShapes[datasource.EntityType(recordType)]
	return served
}

func (archiveRecord) RecordTypeOf(args json.RawMessage) string { return recordTypeArg(args) }

// The STATIC list rather than archivableHere, which needs a context and a
// provider round-trip this seam does not have. The narrower, provider-aware
// refusal still runs in StageInfo and Handle, so a floor entry for a type the
// routed executor will not archive is inert there — the same outcome the wider
// answer produces here.
func (archiveRecord) ServesRecordType(recordType string) bool {
	return slices.Contains(archivableRecordTypes, recordType)
}

func (mergeRecords) RecordTypeOf(args json.RawMessage) string { return recordTypeArg(args) }

func (mergeRecords) ServesRecordType(recordType string) bool { return mergeableTypes[recordType] }

// --- single-type verbs: the contract declares one record type each ---

// The record types a single-type verb answers, named once. They are the
// datasource vocabulary where the seam has a name for the type; import_run is a
// module's own record with no datasource entity, so it is stated here.
const (
	typeLead      = string(datasource.EntityLead)
	typeProject   = string(datasource.EntityProject)
	typeActivity  = string(datasource.EntityActivity)
	typeDeal      = string(datasource.EntityDeal)
	typeImportRun = "import_run"
)

func (promoteLead) RecordTypeOf(json.RawMessage) string          { return typeLead }
func (disqualifyLead) RecordTypeOf(json.RawMessage) string       { return typeLead }
func (advanceProjectPhase) RecordTypeOf(json.RawMessage) string  { return typeProject }
func (commitImport) RecordTypeOf(json.RawMessage) string         { return typeImportRun }
func (sendEmailTool) RecordTypeOf(json.RawMessage) string        { return typeActivity }
func (sendMessageTool) RecordTypeOf(json.RawMessage) string      { return typeActivity }
func (sendAccountEmailTool) RecordTypeOf(json.RawMessage) string { return typeActivity }
func (bookMeetingTool) RecordTypeOf(json.RawMessage) string      { return typeActivity }

func (promoteLead) ServesRecordType(recordType string) bool     { return recordType == typeLead }
func (disqualifyLead) ServesRecordType(recordType string) bool  { return recordType == typeLead }
func (commitImport) ServesRecordType(recordType string) bool    { return recordType == typeImportRun }
func (sendEmailTool) ServesRecordType(recordType string) bool   { return recordType == typeActivity }
func (sendMessageTool) ServesRecordType(recordType string) bool { return recordType == typeActivity }
func (bookMeetingTool) ServesRecordType(recordType string) bool { return recordType == typeActivity }

func (advanceProjectPhase) ServesRecordType(recordType string) bool {
	return recordType == typeProject
}

func (sendAccountEmailTool) ServesRecordType(recordType string) bool {
	return recordType == typeActivity
}

// --- single-type verbs whose tier is resolved dynamically (see the note above) ---

func (advanceDeal) RecordTypeOf(json.RawMessage) string      { return typeDeal }
func (progressDeal) RecordTypeOf(json.RawMessage) string     { return typeDeal }
func (relinkActivity) RecordTypeOf(json.RawMessage) string   { return typeActivity }
func (relinkThread) RecordTypeOf(json.RawMessage) string     { return typeActivity }
func (relinkActivities) RecordTypeOf(json.RawMessage) string { return typeActivity }

func (advanceDeal) ServesRecordType(recordType string) bool      { return recordType == typeDeal }
func (progressDeal) ServesRecordType(recordType string) bool     { return recordType == typeDeal }
func (relinkActivity) ServesRecordType(recordType string) bool   { return recordType == typeActivity }
func (relinkThread) ServesRecordType(recordType string) bool     { return recordType == typeActivity }
func (relinkActivities) ServesRecordType(recordType string) bool { return recordType == typeActivity }
