// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// Which producers answer for the ACTING USER only, whatever scope was asked.
//
// `team` and `all` widen the record-bearing sources, because a wider row scope
// is what reaches a colleague's deal. They cannot widen a source bound to the
// reader inside the module that owns it — notices filter on the recipient, the
// health lanes refuse a principal with no human behind it, and an introduction
// ask names one colleague, so there is no wider tier for it to widen TO.
//
// WHAT THIS CANNOT SEE, stated rather than discovered. The flag is a claim
// about the module behind each source, and the tests beside it hold it against
// that module's own declaration — its port doc — not against its behaviour. A
// per-user lane that started widening, or a record-bearing one that stopped,
// would leave this reporting the old answer about a read the caller cannot
// check. Closing that needs a case per source: a row belonging to somebody
// else, read at `all`, asserted present or absent.
//
// EVERY source is listed, including the ones that widen. A map with a default
// would answer `false` for a producer added tomorrow, which is the one wrong
// direction here: it would tell a reader a source widened when nobody had
// decided whether it does. The test beside this requires the key set to equal
// the contract's own enum, so a new source fails here rather than defaulting to
// a claim about itself.
var sourceAnswersForTheActorOnly = map[crmcontracts.WorklistItemSource]bool{
	// Bound to the reader inside the owning module. Each of these ports says so
	// in its own doc: the health lanes and notices refuse a principal with no
	// human behind them, and their seams take no owner argument because the
	// read binds to the acting human.
	"ai_work_health": true,
	"bounce":         true,
	crmcontracts.WorklistItemSource(sourceCaptureHealth):  true,
	crmcontracts.WorklistItemSource(sourceDomainQuestion): true,
	"failed_approval":                             true,
	"introduction_request":                        true,
	crmcontracts.WorklistItemSource(sourceNotice): true,
	"undelivered":                                 true,
	sourceWeeklyCommitment:                        true,

	// Record-bearing: a wider row scope reaches more of them, which is what
	// `team` and `all` are for.
	"approval":           false,
	"automation_run":     false,
	sourceBriefItem:      false,
	sourceClaim:          false,
	sourceWaiting:        false,
	sourceAtRisk:         false,
	sourceDealSuggestion: false,
	sourceDuplicate:      false,
	"dsr":                false,
	sourceLeadResponse:   false,
	sourceMeeting:        false,
	sourceMeetingOutcome: false,
	sourceNoticeCase:     false,
	"relationship_decay": false,
	sourceTask:           false,

	// A fold stands for its members and carries no reading of its own. Marking
	// it either way would describe rows it only represents.
	"batch": false,
}
