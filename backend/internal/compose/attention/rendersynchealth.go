// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The sync-health lane's card: one per CONDITION, aggregated by the owning
// module, so a broken connector is a single card rather than a flood.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// syncItem draws one sync concern. The card carries no subject and no verbs:
// its subject is the CONNECTION, not a record with a page, and fixing it lives
// on the sync settings screen. `kind` names the condition; `detail` carries
// that condition's facts in the producer's own vocabulary — the affected
// object classes, the failure class, or the budget band — and the client
// writes the sentence in the reader's language.
//
// The id is the concern's kind: a concern is a condition, not a row, and the
// lane carries at most one card per condition.
func syncItem(concern SyncConcern) crmcontracts.AttentionItem {
	kind := concern.Kind
	item := crmcontracts.AttentionItem{
		Id:      concern.Kind,
		Source:  crmcontracts.AttentionItemSource("sync_health"),
		Kind:    &kind,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	switch {
	case len(concern.Objects) > 0:
		classes := strings.Join(concern.Objects, ", ")
		item.Detail = &classes
	case concern.ErrorClass != "":
		errorClass := concern.ErrorClass
		item.Detail = &errorClass
	case concern.Band != "":
		band := concern.Band
		item.Detail = &band
	}
	return item
}

// captureItem draws one capture concern. Like the sync card it carries no
// subject and no verbs — fixing a mailbox lives on the capture settings
// screen. `kind` names the condition; `detail` names the mailbox in the
// reader's own terms: the account label the connector reported, or the
// provider where none was.
func captureItem(concern CaptureConcern) crmcontracts.AttentionItem {
	kind := concern.Kind
	mailbox := concern.AccountLabel
	if mailbox == "" {
		mailbox = concern.Provider
	}
	item := crmcontracts.AttentionItem{
		Id:      concern.ConnectionID.String(),
		Source:  crmcontracts.AttentionItemSource("capture_health"),
		Kind:    &kind,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	if mailbox != "" {
		item.Detail = &mailbox
	}
	return item
}
