// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// A link the run drops is said in two tenses. A test run has written nothing,
// so its row will be imported without the link; a run that has written says
// the row was. Each tense is its own code, so a screen words either one.

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for a link a test run says it will drop.
const (
	ReasonLinkNotFoundWillDrop          ReasonCode = "link_not_found_will_drop"
	ReasonContactNotFoundWillDrop       ReasonCode = "contact_not_found_will_drop"
	ReasonLinkAmbiguousWillDrop         ReasonCode = "link_ambiguous_will_drop"
	ReasonEmployerColumnEmptyWillDrop   ReasonCode = "employer_column_empty_will_drop"
	ReasonEmployerNotFoundWillDrop      ReasonCode = "employer_not_found_will_drop"
	ReasonLeadNotFoundWillDrop          ReasonCode = "lead_not_found_will_drop"
	ReasonDealNotFoundWillDrop          ReasonCode = "deal_not_found_will_drop"
	ReasonLinkRowSkippedWillDrop        ReasonCode = "link_row_skipped_will_drop"
	ReasonActivityLinkAmbiguousWillDrop ReasonCode = "activity_link_ambiguous_will_drop"
)

// willDropTwin names, for each dropped-link code, the code that says the same
// before the run has written.
var willDropTwin = map[ReasonCode]ReasonCode{
	ReasonLinkNotFoundDropped:          ReasonLinkNotFoundWillDrop,
	ReasonContactNotFoundDropped:       ReasonContactNotFoundWillDrop,
	ReasonLinkAmbiguousDropped:         ReasonLinkAmbiguousWillDrop,
	ReasonEmployerColumnEmptyDropped:   ReasonEmployerColumnEmptyWillDrop,
	ReasonEmployerNotFoundDropped:      ReasonEmployerNotFoundWillDrop,
	ReasonLeadNotFoundDropped:          ReasonLeadNotFoundWillDrop,
	ReasonDealNotFoundDropped:          ReasonDealNotFoundWillDrop,
	ReasonLinkRowSkippedDropped:        ReasonLinkRowSkippedWillDrop,
	ReasonActivityLinkAmbiguousDropped: ReasonActivityLinkAmbiguousWillDrop,
}

// droppedTense is the sentence every dropped-link reason ends with, once the
// run has written and before.
var droppedTense = map[textlang.Lang][2]string{
	textlang.English:    {"The row was imported without the link.", "The row will be imported without the link."},
	textlang.German:     {"Die Zeile wurde ohne Verknüpfung importiert.", "Die Zeile wird ohne Verknüpfung importiert."},
	textlang.Vietnamese: {"Hàng đã được nhập mà không có liên kết.", "Hàng sẽ được nhập mà không có liên kết."},
}

// withWillDropReasons adds each test-run twin as its dropped-link reason with
// the last sentence in the other tense, so the two never word the cause apart.
func withWillDropReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	for dropped, willDrop := range willDropTwin {
		written := catalog[dropped]
		text := make(map[textlang.Lang]string, len(written.text))
		for lang, sentence := range written.text {
			tense := droppedTense[lang]
			text[lang] = strings.Replace(sentence, tense[0], tense[1], 1)
		}
		catalog[willDrop] = reasonEntry{params: written.params, text: text}
	}
	return catalog
}

// Told is the reason in the tense of its run. A dropped link reads as done once
// the run has written, and as still to come before.
func (r Reason) Told(written bool) Reason {
	for dropped, willDrop := range willDropTwin {
		switch {
		case written && r.Code == willDrop:
			r.Code = dropped
		case !written && r.Code == dropped:
			r.Code = willDrop
		}
	}
	return r
}
