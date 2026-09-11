// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package hubspot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// This file is the write direction of the HubSpot mapping-as-contract
// (OVA-MAP-W): the canonical→incumbent projection the write-back engine
// PATCHes/POSTs to HubSpot. It is pinned SEPARATELY from the read mapping
// (mapping_hs.go) rather than inferred from it, because the read projection
// (OVA-MAP-1..8) is not a bijection — several rules are derived or lossy
// one-way (assembled full_name, currency-scaled amounts, class-derived
// activity kind, association-derived lead fields, lossy size_band). A
// canonical field with no writable incumbent counterpart is READ-ONLY: it is
// never written back, and a write carrying only read-only fields is a no-op
// the caller is told about (empty Props), never a fabricated incumbent
// property.

// writeMapping is one canonical→HubSpot write projection: the incumbent
// object class the write targets and the HubSpot properties to set. Props
// carries only WRITABLE properties — a canonical field flagged read-only by
// OVA-MAP-W (full_name, occurred_at, lead email/company_name/status, deal
// pipeline_id/stage_id, company size_band, activity meeting_status) never
// appears. An empty Props on a CREATE means the write touched only read-only
// fields (the caller is told); on an UPDATE it means the patch changed
// nothing writable.
type writeMapping struct {
	ObjectClass string
	Props       map[string]string
	// Dropped is the canonical fields the caller asked to write that this
	// projection cannot send, in name order. It is what turns a silent drop
	// into one somebody can see: the write still succeeds — the incumbent has
	// the fields it can hold — but an operator reading the log knows which
	// values did not leave, instead of learning it from a user reporting that
	// an edit reverted itself.
	Dropped []string
}

// mapWrite projects a canonical write (the entity type + the canonical field
// bag, the JSON-decoded contract the frozen datasource seam carries) onto the
// HubSpot object class and property set per OVA-MAP-W1..6. forUpdate selects
// clear-semantics: on an update an explicit "" clears the incumbent property
// (HubSpot's documented clear), while on a create an empty value is simply
// nothing to set. It is pure: no I/O, no transport — the golden write-mapping
// suite (AC-OV-12) exercises it directly.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag from the frozen datasource seam; the any is inherent to the decoded shape
func mapWrite(canonicalClass string, fields map[string]any, forUpdate bool) (writeMapping, error) {
	switch canonicalClass {
	case personTarget:
		props, err := copyDirect(fields, personWriteFields, forUpdate)
		return writeMapping{
			ObjectClass: objectClassContacts, Props: props,
			Dropped: droppedFields(fields, deferredPersonWrites),
		}, err
	case companyTarget:
		props, err := copyDirect(fields, companyWriteFields, forUpdate)
		return writeMapping{
			ObjectClass: objectClassCompanies, Props: props,
			Dropped: droppedFields(fields, deferredCompanyWrites),
		}, err
	case leadTarget:
		props, err := copyDirect(fields, leadWriteFields, forUpdate)
		return writeMapping{
			ObjectClass: objectClassLeads, Props: props,
			Dropped: droppedFields(fields, deferredLeadWrites),
		}, err
	case dealTarget:
		return mapWriteDeal(fields, forUpdate)
	case activityTarget:
		return mapWriteActivity(fields, forUpdate)
	default:
		return writeMapping{}, fmt.Errorf("overlay: no HubSpot write mapping for canonical class %q", canonicalClass)
	}
}

// directWriteField is one canonical field that projects 1:1 onto a HubSpot
// property by a straight string copy. The read-only canonical fields simply
// have no entry — that absence is what makes them never written.
// canonicalOwnerID is the canonical field every class defers for the same
// reason: writing it back needs the reverse owner user-map resolution, which
// resolves a Margince user to the incumbent's own owner id.
const canonicalOwnerID = "owner_id"

type directWriteField struct {
	Canonical string
	HSProp    string
}

// personWriteFields — OVA-MAP-W1. full_name is the assembled display field
// (OVA-MAP-3) and is NOT here: splitting a display string into first/last is
// ambiguous and lossy, so it is read-only. emails, address, and owner_id are
// V1 write-back deferrals (the emails child, the structured-address
// assembler, and the reverse owner user-map resolution have no simple 1:1
// inverse yet) — read-only for now, surfaced honestly rather than guessed,
// the same "flag, don't invent" posture the read mapping takes for phone/social.
var personWriteFields = []directWriteField{
	{Canonical: "first_name", HSProp: propFirstname},
	{Canonical: "last_name", HSProp: propLastname},
	{Canonical: "title", HSProp: "jobtitle"},
}

// deferredPersonWrites — the same obligation as deferredCompanyWrites, for
// the fields updatePerson lets a caller send.
var deferredPersonWrites = map[string]string{
	"full_name": "the assembled display field (OVA-MAP-3): splitting a display string back into " +
		"first/last is ambiguous and lossy, and first_name/last_name above already carry the parts",
	"emails":         "a 1:N child collection, not a contact property — the inverse needs the child writer",
	"phones":         "the same 1:N child shape as emails",
	"social":         "a 1:N child collection, and HubSpot's social properties are per-network singletons",
	targetAddress:    "the structured-address assembler has no simple 1:1 inverse yet",
	canonicalOwnerID: "needs the reverse owner user-map resolution, canonical user → incumbent owner id",
	"visibility": "capture privacy is a Margince access-control property, not a fact about the contact — " +
		"the incumbent has no counterpart and must not be told who may see a row here",
}

// companyWriteFields — the inverse of companiesMapping's 1:1 columns.
var companyWriteFields = []directWriteField{
	{Canonical: "display_name", HSProp: propName},
	{Canonical: industryField, HSProp: industryField},
}

// deferredCompanyWrites is every field a caller may PATCH on an
// company that this projection does NOT carry, each with the reason.
//
// It exists because the alternative is silence, and silence here is the defect:
// a canonical field absent from the projection is accepted by the door, audited,
// emitted as an update — and never sent. The next overlay read returns the old
// value, so the edit looks to a user like somebody else reverted it.
//
// Three of these were not deferrals at all until this list existed. legal_name,
// linkedin_url and description were simply never added when their columns
// landed, and three separate column changes each failed to notice the same
// obligation — which is one missing rule rather than three mistakes.
// Held by: TestEveryOverlayWritableFieldProjectsOrSaysWhyNot
// (backend/gates/overlaywritecoverage_test.go) — a field the contract lets a
// caller write is either here or in the projection above, and the same rule
// answers for person and lead.
var deferredCompanyWrites = map[string]string{
	"size_band": "read-only: numberofemployees→size_band is a lossy band bucketing " +
		"(employees_to_size_band) with no unambiguous inverse — writing back the band's floor would " +
		"report a headcount nobody stated",
	targetAddress:    "the structured-address assembler has no simple 1:1 inverse yet (the V1 write-back deferral)",
	"domains":        "a 1:N child collection, not a company property — the inverse needs the child writer",
	canonicalOwnerID: "needs the reverse owner user-map resolution, canonical user → incumbent owner id",
	"legal_name": "HubSpot declares no counterpart property, so there is nothing to be the inverse OF. " +
		"Projecting it onto a custom property would invent a mapping the read side does not make",
	"linkedin_url": "the READ side does not carry it either (companiesMapping maps no LinkedIn property), " +
		"so a write alone would push a value the next read cannot bring back — the field would oscillate. " +
		"Both halves land together; the read half is issue #1027",
	"description": "same shape as linkedin_url, plus a real choice: `about_us` and the CRM `description` " +
		"property are both candidates and behave differently, and the write must be the inverse of " +
		"whichever the read takes. The read half is issue #1026",
	"lifecycle": "HubSpot's lifecyclestage names a different axis from our lifecycle and the two " +
		"vocabularies do not correspond term for term; issue #1028 holds the read half and the transform " +
		"both directions would need",
	"parent_company_id":  "a company-to-company association, not a property — it writes through the association API rather than through this projection",
	"relationship_types": "a Margince concept with no HubSpot counterpart: the incumbent models no such classification",
}

// leadWriteFields — OVA-MAP-W5's writable Leads-object property that has a
// clean, transform-free projection: full_name → hs_lead_name.
//
// The status ↔ hs_lead_label projection is DEFERRED in both directions, on
// purpose: the read side (OVA-MAP-5, leadsMapping) keeps hs_lead_label a RAW
// passthrough and explicitly defers the typed status-enum remap "until a
// documented transform + a real capture". Writing a canonical status enum
// token straight into HubSpot's hs_lead_label would be exactly that
// untransformed projection the read declined as unsafe — and it would not be
// the inverse of the read (which produces hs_lead_label, never status). So
// lead status/label write-back waits on a pinned, bidirectional value map;
// this is a contract-first reconciliation item against OVA-MAP-W5, which
// currently assumes a transform OVA-MAP-5 has not defined.
//
// email and company_name are DERIVED through the required contact association
// (OVA-MAP-5), not Leads-object properties Margince can write, so they are
// read-only and absent here too.
var leadWriteFields = []directWriteField{
	{Canonical: targetFullName, HSProp: "hs_lead_name"},
}

// deferredLeadWrites — the same obligation, for the fields updateLead lets a
// caller send. The status/label reasoning is in leadWriteFields' own comment.
var deferredLeadWrites = map[string]string{
	"status": "waits on a pinned bidirectional value map: the read keeps hs_lead_label a RAW passthrough, " +
		"so writing a canonical status token into it would be the untransformed projection the read declined",
	"email":                 "DERIVED through the required contact association, not a Leads-object property Margince can write",
	"company_name":          "derived the same way, through the association rather than a property",
	canonicalOwnerID:        "needs the reverse owner user-map resolution",
	"title":                 "a property of the associated CONTACT, not of the Leads object",
	"source":                "no Leads-object counterpart; the incumbent records lead provenance differently",
	"score":                 "a Margince-computed figure, and writing it would publish our model's output as though the incumbent had produced it",
	"score_override_reason": "the human sentence explaining a score override — same reason as score",
	"project_id":            "a Margince association with no Leads-object counterpart",
	"candidate_company_key": "a matching key internal to our own resolution, never an incumbent property",
}

// stringProp reads a canonical STRING field's writable value. present reports
// whether the key is present with a non-null value (an explicit "" is present
// — the clear-field signal on updates). A present non-string value is a type
// error, matching the native provider's StrictDecode 422 rather than
// coercing a number/bool into a HubSpot string property.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func stringProp(fields map[string]any, key string) (val string, present bool, err error) {
	raw, ok := fields[key]
	if !ok || raw == nil {
		return "", false, nil
	}
	s, isStr := raw.(string)
	if !isStr {
		return "", true, fmt.Errorf("overlay: field %q must be a string, got %T", key, raw)
	}
	return s, true, nil
}

// putString sets props[hsProp] from the canonical string field, honoring the
// create/update clear-field rule: an explicit "" clears on update (sent) and
// is skipped on create (nothing to set). Returns an error on a type violation.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func putString(props map[string]string, fields map[string]any, canonical, hsProp string, forUpdate bool) error {
	// An empty hsProp means the object class has no such writable property
	// (e.g. a note has no subject) — the canonical field is read-only for this
	// class and skipped, even though it is a valid contract field.
	if hsProp == "" {
		return nil
	}
	val, present, err := stringProp(fields, canonical)
	if err != nil {
		return err
	}
	if !present || (val == "" && !forUpdate) {
		return nil
	}
	props[hsProp] = val
	return nil
}

// copyDirect projects the direct 1:1 fields onto their HubSpot property names.
// A canonical field that is absent or JSON-null is not written; an explicit
// "" clears on update; a field not in the table is read-only and skipped.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func copyDirect(fields map[string]any, table []directWriteField, forUpdate bool) (map[string]string, error) {
	props := make(map[string]string)
	for _, f := range table {
		if err := putString(props, fields, f.Canonical, f.HSProp, forUpdate); err != nil {
			return nil, err
		}
	}
	return props, nil
}

// droppedFields is the canonical fields this write ASKED for that the
// projection cannot send, named so a caller of mapWrite can say so.
//
// The deferral maps are the source, which is what keeps them honest: a field
// deferred with a reason is a field this function reports, so the reasons are
// load-bearing rather than a comment that drifts. A patch naming only projected
// fields answers nil.
func droppedFields(fields map[string]any, deferred map[string]string) []string {
	var dropped []string
	for field := range fields {
		if _, isDeferred := deferred[field]; isDeferred {
			dropped = append(dropped, field)
		}
	}
	sort.Strings(dropped)
	return dropped
}

// mapWriteDeal — OVA-MAP-W2. name/expected_close_date project directly;
// amount_minor + currency scale BACK to the decimal amount string by the
// ISO-4217 minor-unit exponent of currency (never a blanket ÷100), and
// currency sets deal_currency_code. pipeline_id/stage_id are read-only in
// overlay (null per OVA-MAP-6/W4) and never written.
//
// amount and currency are a PAIR: a caller must supply currency to write
// amount (the exponent chooses the decimal point). Provider.Update refuses a
// patch carrying one without the other before ever reaching this mapping, so
// the half-pair never arrives here; the guard below is the second line —
// an amount_minor with no currency writes no amount rather than guess an
// exponent.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func mapWriteDeal(fields map[string]any, forUpdate bool) (writeMapping, error) {
	props, err := copyDirect(fields, dealWriteFields, forUpdate)
	if err != nil {
		return writeMapping{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(stringField(fields, "currency")))
	if currency != "" {
		props["deal_currency_code"] = currency
	}
	minor, ok, err := numericField(fields, "amount_minor")
	if err != nil {
		return writeMapping{}, err
	}
	if ok && currency != "" {
		// values.MajorUnits is the one renderer of a minor-unit integer as a
		// currency-decimal string, and HubSpot's `amount` property wants exactly
		// that shape (exponent 0 → no point; 2 → "10.00"; 3 → "1.500"), never a
		// blanket ÷100. This used to be a second implementation here; the two
		// agreed on all 272 amount×currency pairs they were compared over,
		// which is precisely how long a second copy stays harmless.
		props["amount"] = values.MajorUnits(minor, currency)
	}
	return writeMapping{ObjectClass: objectClassDeals, Props: props}, nil
}

var dealWriteFields = []directWriteField{
	{Canonical: "name", HSProp: "dealname"},
	{Canonical: "expected_close_date", HSProp: "closedate"},
}

// mapWriteActivity — OVA-MAP-W3. kind selects the v3 engagement object class
// the write targets (the inverse of OVA-MAP-1's class→kind); each class has
// its own subject/body/direction property names. duration_seconds →
// hs_call_duration ×1000 (ms, call only); a task's due_at → hs_timestamp
// (task only). occurred_at is read-only (OVA-MAP-8) and never written.
// meeting_status is read-only: HubSpot's hs_meeting_outcome is a pinned
// enum vocabulary, not the canonical status string, so — like lead status —
// it awaits a documented bidirectional value map rather than sending a raw
// canonical token.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func mapWriteActivity(fields map[string]any, forUpdate bool) (writeMapping, error) {
	kind := strings.TrimSpace(stringField(fields, "kind"))
	if kind == "" {
		return writeMapping{}, fmt.Errorf("overlay: an activity write carries no kind — cannot choose a HubSpot engagement object class")
	}
	spec, ok := activityWriteSpecs[kind]
	if !ok {
		return writeMapping{}, fmt.Errorf("overlay: activity kind %q has no HubSpot engagement object class", kind)
	}
	props := make(map[string]string)
	if err := putString(props, fields, targetSubject, spec.subjectProp, forUpdate); err != nil {
		return writeMapping{}, err
	}
	if err := putString(props, fields, targetBody, spec.bodyProp, forUpdate); err != nil {
		return writeMapping{}, err
	}
	if err := applyActivitySpecials(props, fields, spec, forUpdate); err != nil {
		return writeMapping{}, err
	}
	return writeMapping{ObjectClass: spec.objectClass, Props: props}, nil
}

// applyActivitySpecials projects the per-class activity fields that are not a
// plain string copy: direction (uppercased to HubSpot's pinned INBOUND/
// OUTBOUND enum, the inverse of the lowercase canonical token), duration
// (seconds → milliseconds), and a task's due_at (RFC3339 → epoch millis). A
// class that does not carry a field has an empty/false spec entry and is
// skipped.
//
//craft:ignore naked-any fields is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func applyActivitySpecials(props map[string]string, fields map[string]any, spec activityWriteSpec, forUpdate bool) error {
	if spec.directionProp != "" {
		val, present, err := stringProp(fields, targetDirection)
		if err != nil {
			return err
		}
		if present && (val != "" || forUpdate) {
			props[spec.directionProp] = strings.ToUpper(val)
		}
	}
	if spec.hasDuration {
		secs, ok, err := numericField(fields, "duration_seconds")
		if err != nil {
			return err
		}
		if ok {
			props["hs_call_duration"] = strconv.FormatInt(secs*1000, 10)
		}
	}
	if spec.hasDueAt {
		ms, ok, err := rfc3339ToMillis(fields["due_at"])
		if err != nil {
			return err
		}
		if ok {
			props[propHSTimestamp] = ms
		}
	}
	return nil
}

// activityWriteSpec is one engagement class's write projection — the inverse
// of the class's read ObjectMapping. An empty property name means the class
// does not carry that field. meeting_status is deliberately absent (deferred,
// see mapWriteActivity).
type activityWriteSpec struct {
	objectClass   string
	subjectProp   string
	bodyProp      string
	directionProp string
	hasDuration   bool
	hasDueAt      bool
}

// activityWriteSpecs maps each canonical activity kind to its engagement
// class's write projection, matching the read property names in
// mapping_hs.go (calls/meetings/emails/notes/tasks).
var activityWriteSpecs = map[string]activityWriteSpec{
	kindCall:    {objectClass: objectClassCalls, subjectProp: "hs_call_title", bodyProp: "hs_call_body", directionProp: "hs_call_direction", hasDuration: true},
	kindMeeting: {objectClass: objectClassMeetings, subjectProp: "hs_meeting_title", bodyProp: "hs_meeting_body"},
	kindEmail:   {objectClass: objectClassEmails, subjectProp: "hs_email_subject", bodyProp: "hs_email_text", directionProp: "hs_email_direction"},
	kindNote:    {objectClass: objectClassNotes, bodyProp: "hs_note_body"},
	kindTask:    {objectClass: objectClassTasks, subjectProp: "hs_task_subject", bodyProp: "hs_task_body", hasDueAt: true},
}

// stringField reads a string-valued canonical property, answering "" for an
// absent, JSON-null, or non-string value — a local convenience for the plain
// selector fields (currency, kind) mapWrite branches on before deciding a
// projection.
//
//craft:ignore naked-any v is the JSON-decoded canonical bag; the any is inherent to the decoded shape
func stringField(fields map[string]any, key string) string {
	if s, ok := fields[key].(string); ok {
		return s
	}
	return ""
}

// numericField reads an integer-valued canonical field WITHOUT the precision
// loss of a float64 round-trip: the write path decodes the bag with
// json.Decoder.UseNumber (canonicalFields), so a number arrives as a
// json.Number and is parsed as a strict, range-checked int64. A plain float64
// (the unit tests that build the bag directly) is accepted only when it is an
// exact integer in int64 range — a fractional or out-of-range value is a type
// error, never a silent truncation. A null/absent value is ok=false, nil error.
//
//craft:ignore naked-any v is the JSON-decoded canonical value; the any is inherent to the decoded shape
func numericField(fields map[string]any, key string) (int64, bool, error) {
	switch t := fields[key].(type) {
	case nil:
		return 0, false, nil
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0, true, fmt.Errorf("overlay: field %q is not an int64: %w", key, err)
		}
		return n, true, nil
	case int64:
		return t, true, nil
	case int:
		return int64(t), true, nil
	case float64:
		if t != float64(int64(t)) {
			return 0, true, fmt.Errorf("overlay: field %q must be an integer, got %v", key, t)
		}
		return int64(t), true, nil
	default:
		return 0, true, fmt.Errorf("overlay: field %q must be a number, got %T", key, fields[key])
	}
}

// rfc3339ToMillis parses a canonical RFC3339 timestamp (how a contract
// time.Time marshals to JSON) into a HubSpot epoch-millis property string. A
// null/absent value is ("", false, nil) — nothing to write. A present but
// unparseable value is an error, never a silently dropped timestamp.
//
//craft:ignore naked-any v is a JSON-decoded canonical value; the any is inherent to the decoded shape
func rfc3339ToMillis(v any) (string, bool, error) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", false, nil
	}
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "", false, fmt.Errorf("overlay: write timestamp %q is not RFC3339: %w", s, err)
	}
	return strconv.FormatInt(ts.UnixMilli(), 10), true, nil
}
