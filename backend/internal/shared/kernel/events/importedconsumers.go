// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// What each consumer group does with an event an import wrote.
//
// An import brings records in with their history. Reacting to that as news
// would start automations on old moves and accrue commissions on sales paid for
// elsewhere. So every group that reads the deal or activity stream states here
// whether it acts on an imported event, and the subscriber enforces the answer.
// An event is imported when its payload carries import_run_id.

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// ImportedPolicy is what a consumer group does with an imported event.
type ImportedPolicy int

const (
	// SkipImported leaves the event unhandled and acknowledged: the group
	// reacts to something that happens now, and an import is not that.
	SkipImported ImportedPolicy = iota + 1
	// KeepImported delivers the event as any other: the group projects,
	// records or forwards it, and an imported record belongs there too.
	KeepImported
)

// ImportedDecision is a group's policy and the reason it holds.
type ImportedDecision struct {
	Policy ImportedPolicy
	Reason string
}

// ExtensionGroupPrefix starts the name of every extension unit's consumer
// group. A unit's listener reacts to what happens in the workspace, so the
// default for the whole tier is to leave imported events out.
const ExtensionGroupPrefix = "cg:ext-"

var importedDecisions = map[string]ImportedDecision{
	"cg:deal-refile":        {KeepImported, "recent captured correspondence follows imported deal and contact facts through the same attribution rule"},
	"cg:context-graph":      {KeepImported, "embeds and indexes the record, and imported history belongs in the graph"},
	"cg:read-model":         {KeepImported, "a projection holds every deal, however it arrived"},
	"cg:audit-stream":       {KeepImported, "the audit slice records every event"},
	"cg:webhooks":           {KeepImported, "outside systems decide for themselves, and the payload says the event was imported"},
	"cg:commissions":        {SkipImported, "a sale an import brings in was priced and paid in the system it came from"},
	"cg:partner-activation": {SkipImported, "a sale an import brings in happened in the past, and activation is a fact of today"},
	"cg:stage-evidence":     {SkipImported, "evidence settles what happened now, and imported history is not a new fact"},
	"cg:deal-room-timeline": {SkipImported, "writes a note about a room that is live today"},
	"cg:chat-links":         {SkipImported, "posts what changes today into a live chat channel"},
	"cg:overnight-agent":    {SkipImported, "plans work against the live pipeline, and closed history is not work"},
	"cg:workflows":          {SkipImported, "an automation starts on a change somebody makes, never on a bulk import"},
	"cg:flow-bridge":        {SkipImported, "hands live events to the flow engine, which acts on them as news"},
	"cg:graph-edge":         {KeepImported, "the interaction graph is built from history, so imported activity belongs in it"},
	"cg:audience-rescope":   {KeepImported, "narrows derived signals when a message is limited, which is as true of an old message"},
	"cg:voice-copy-recheck": {KeepImported, "empties a voice sample when its mail stops being learnable, which is as true of an old mail"},
	"cg:intro-advance":      {SkipImported, "an old reply must not close an introduction asked for today"},
	"cg:notice-case-open":   {KeepImported, "a mailbox import reads a contact's own mail after the reply that minted them, and that mail settles the notice duty"},
	"cg:commitment-settle":  {SkipImported, "an old task update must not settle a commitment kept today"},
	"cg:capture-enrich":     {KeepImported, "defers imported mailbox history to the end of its run; the handler itself leaves out other imports"},
	"cg:vcard-ingest":       {KeepImported, "a card attached to an old mail still states the sender's details"},
	"cg:assurance-freshness": {
		KeepImported, "an imported deal or activity is a record the last input check has not read",
	},
}

// ImportedDecisionOf answers a group's decision. A group with no entry has none,
// which a gate turns into a failure for every group reading an imported stream.
func ImportedDecisionOf(group string) (ImportedDecision, bool) {
	if strings.HasPrefix(group, ExtensionGroupPrefix) {
		return ImportedDecision{SkipImported, "a unit reacts to what happens in the workspace, not to history an import brought in"}, true
	}
	decision, ok := importedDecisions[group]
	return decision, ok
}

// ImportedDecisionGroups names every group with an entry, sorted, so a gate can
// hold the table against the groups the catalog declares.
func ImportedDecisionGroups() []string {
	return slices.Sorted(maps.Keys(importedDecisions))
}

// SkipsImported reports whether the group leaves imported events out.
func SkipsImported(group string) bool {
	decision, ok := ImportedDecisionOf(group)
	return ok && decision.Policy == SkipImported
}

// IsImported reports whether an import wrote the change an event announces.
// Only deal and activity events carry the marker, so only they are read.
func IsImported(env Envelope) bool {
	if len(env.Payload) == 0 || !carriesImportMarker(env.Type) {
		return false
	}
	var marker struct {
		ImportRunID string `json:"import_run_id"`
	}
	if err := json.Unmarshal(env.Payload, &marker); err != nil {
		return false
	}
	return marker.ImportRunID != ""
}

// carriesImportMarker is whether an event type rides a stream imports write.
func carriesImportMarker(eventType string) bool {
	stream, err := StreamFor(eventType)
	return err == nil && ImportedStreams()[stream]
}

// ImportedStreams is the set of streams an import writes events on. A group
// that reads one of them owes a decision.
func ImportedStreams() map[string]bool {
	return map[string]bool{
		StreamPrefix + dealStreamEntity:     true,
		StreamPrefix + activityStreamEntity: true,
	}
}
