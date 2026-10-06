// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package retentionscope spells, once, the SQL fragments that say what keeps an
// activity from being erased or released: a legal hold reached through a link, a
// deal that qualifies it as commercial correspondence, an erasure request that
// covers its contact.
//
// The erasure engine, the retention sweep and the undo of a project filing each
// ask these questions of the same rows, and a module never imports a sibling, so
// without this package each would restate them and the copies would drift.
// Every fragment takes the SQL expression naming the activity, because the
// callers alias it differently, and is stdlib-only text with no placeholders.
package retentionscope

// HeldThroughRecordLinks is true when a legal hold sits on a company, deal, lead
// or project the activity is linked to. The contact arm is separate: a contact
// hold is proven on the erased subject itself before an erasure cascade runs.
func HeldThroughRecordLinks(activityID string) string {
	return `EXISTS (
	    SELECT 1 FROM activity_link h
	    LEFT JOIN company company ON company.id = h.company_id
	    LEFT JOIN deal dl ON dl.id = h.deal_id
	    LEFT JOIN lead ld ON ld.id = h.lead_id
	    LEFT JOIN project pj ON pj.id = h.project_id
	    WHERE h.activity_id = ` + activityID + `
	      AND (coalesce(company.legal_hold, false) OR coalesce(dl.legal_hold, false)
	           OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))`
}

// HeldThroughAnyLink is HeldThroughRecordLinks plus the contact arm.
func HeldThroughAnyLink(activityID string) string {
	return `EXISTS (
	    SELECT 1 FROM activity_link h
	    LEFT JOIN contact hp ON hp.id = h.contact_id
	    LEFT JOIN company company ON company.id = h.company_id
	    LEFT JOIN deal dl ON dl.id = h.deal_id
	    LEFT JOIN lead ld ON ld.id = h.lead_id
	    LEFT JOIN project pj ON pj.id = h.project_id
	    WHERE h.activity_id = ` + activityID + `
	      AND (coalesce(hp.legal_hold, false) OR coalesce(company.legal_hold, false) OR coalesce(dl.legal_hold, false)
	           OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))`
}

// QualifyingDealLink is true when a deal the activity is linked to qualifies it
// as commercial correspondence: won, or carrying an offer past draft. A sent
// offer documents a Handelsgeschäft whether or not the deal closed.
func QualifyingDealLink(activityID string) string {
	return `EXISTS (
	    SELECT 1 FROM activity_link hl
	    JOIN deal hd ON hd.id = hl.deal_id
	    WHERE hl.activity_id = ` + activityID + ` AND hl.entity_type = 'deal'
	      AND (hd.status = 'won'
	           OR EXISTS (SELECT 1 FROM offer o WHERE o.deal_id = hd.id AND o.status <> 'draft')))`
}

// UnderOpenErasure is true when an open erasure request names a contact the
// activity is linked to, by the request's contact or by its reference text.
func UnderOpenErasure(activityID string) string {
	return `EXISTS (
	    SELECT 1 FROM data_subject_request d
	    JOIN activity_link el ON el.activity_id = ` + activityID + ` AND el.contact_id IS NOT NULL
	    WHERE d.kind = 'erasure' AND d.status IN ('open', 'in_progress')
	      AND (el.contact_id = d.contact_id OR el.contact_id::text = d.subject_ref))`
}
