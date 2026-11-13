// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package webformlead lands a website form submission as a lead and a note on
// its timeline, through the lead and activity writers.
package webformlead

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// webFormLeadSource is the lead and note `source` a website form stamps.
const webFormLeadSource = "web_form"

// FieldValue is the value a lead custom field takes from the text a form sent
// for it. It answers false when the text does not fit the field.
//
//craft:ignore naked-any the value is a lead CustomFields entry, shaped by the column type
type FieldValue func(name string, column fieldcatalog.Column, text string) (any, bool)

// Lander lands a website form submission through the lead and activity
// writers. A new email is a new lead. An email an open lead holds gets the
// note on that lead. An email a contact holds gets it on the contact, whose
// owner the lead writer has already told.
type Lander struct {
	leads      *contacts.Store
	activities *activities.Store
	catalog    fieldcatalog.Reader
	legacy     *automation.AutomationStore
	fieldValue FieldValue
}

// New builds the lander. fieldValue reads a cf_<key> field the way the import
// reads a cell, so a form and a spreadsheet accept the same text.
func New(leads *contacts.Store, notes *activities.Store, catalog fieldcatalog.Reader,
	legacy *automation.AutomationStore, fieldValue FieldValue,
) Lander {
	return Lander{leads: leads, activities: notes, catalog: catalog, legacy: legacy, fieldValue: fieldValue}
}

// Land writes the submission's lead, or finds the record that holds its
// address, and puts the form's note on it.
func (l Lander) Land(ctx context.Context, landing webforms.Landing) error {
	active, err := l.catalog.ActiveColumns(ctx, "lead")
	if err != nil {
		return fmt.Errorf("compose: reading the lead custom fields: %w", err)
	}
	in := LeadInput(landing)
	in.CustomFields = webFormCustomFields(active, landing.Enquiry, l.fieldValue)
	target, err := l.lead(ctx, in)
	if err != nil || target == nil {
		return err
	}
	if landing.HandlingAutomationID != nil && target.EntityType == (ids.LeadID{}).EntityType() {
		owned, err := l.legacyOwnsInquiry(ctx, target.EntityID, landing.SourceKey)
		if err != nil {
			return err
		}
		if owned {
			landing.HandlingAutomationID = nil
		}
	}
	system, key := provenance.WebFormSource, landing.SourceKey
	subject := fmt.Sprintf(systemtext.Copy("form.subject", textlang.English), landing.FormName)
	body := webFormNote(landing)
	raw, err := webFormRaw(landing, webFormValidatedPicklists(active, in))
	if err != nil {
		return err
	}
	_, _, err = l.activities.LogActivity(ctx, activities.LogActivityInput{
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("form.subject", systemtext.Label(landing.FormName)), Body: webFormBodyComponent(landing, body)},
		Kind:       string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Links: []activities.ActivityLinkInput{*target},
		SourceSystem: &system, SourceID: &key, Source: webFormLeadSource, Raw: &raw,
	})
	if err != nil {
		return fmt.Errorf("compose: writing the website form's note: %w", err)
	}
	return nil
}

// lead answers the record the note belongs on. Nil with no error means nothing
// more is written. That is a contact this principal may not read, whose owner
// has a task about it, or an address on the erasure list.
func (l Lander) lead(ctx context.Context, in contacts.CreateLeadInput) (*activities.ActivityLinkInput, error) {
	lead, err := l.createLead(ctx, in)
	if duplicate, ok := errors.AsType[*contacts.DuplicateLeadError](err); ok {
		return &activities.ActivityLinkInput{EntityType: (ids.LeadID{}).EntityType(), EntityID: duplicate.ExistingID.UUID}, nil
	}
	if held, ok := errors.AsType[*contacts.ContactHoldsIdentityError](err); ok {
		if held.ContactID.IsZero() {
			return nil, nil //nolint:nilnil // a contact this principal may not read has no note target, and that is no failure
		}
		return &activities.ActivityLinkInput{EntityType: (ids.ContactID{}).EntityType(), EntityID: held.ContactID.UUID}, nil
	}
	if _, erased := errors.AsType[*storekit.ErasedIdentifierError](err); erased {
		// The visitor gets the receipt every post gets. A refusal would tell an
		// anonymous sender that the address belongs to an erased subject.
		return nil, nil //nolint:nilnil // an erased address has no note target, and that is no failure
	}
	if err != nil {
		return nil, err
	}
	return &activities.ActivityLinkInput{EntityType: (ids.LeadID{}).EntityType(), EntityID: ids.UUID(lead.Id)}, nil
}

// createLead lands the lead without whatever the form names that has gone since
// it was saved. A visitor is never refused over the form's own setup. Each retry
// clears one thing, and any of them can be the one refused first.
func (l Lander) createLead(ctx context.Context, in contacts.CreateLeadInput) (crmcontracts.Lead, error) {
	for {
		lead, _, err := l.leads.CreateLead(ctx, in)
		_, fieldGone := errors.AsType[storekit.CustomFieldErrors](err)
		_, ownerGone := errors.AsType[*auth.AssigneeNotAllowedError](err)
		switch {
		case fieldGone && in.CustomFields != nil:
			// The values are on the note, so the lead lands without them.
			in.CustomFields = nil
		case errors.Is(err, contacts.ErrLeadProductRetired) && in.ProductID != nil:
			in.ProductID = nil
		case ownerGone && in.OwnerID != nil:
			// The unassigned queue takes the lead of a deactivated owner.
			in.OwnerID = nil
		default:
			return lead, err
		}
	}
}

// LeadInput is the lead a submission lands as, before its custom fields.
func LeadInput(landing webforms.Landing) contacts.CreateLeadInput {
	e := landing.Enquiry
	system, key := provenance.WebFormSource, landing.SourceKey
	in := contacts.CreateLeadInput{
		Email: &e.Email, FullName: optional(e.FullName), Title: optional(e.Title),
		CompanyName: optional(e.Company), SourceSystem: &system, SourceID: &key,
		Source: webFormLeadSource, Origin: landing.LeadOrigin, OriginDetail: optional(originDetail(e)),
	}
	if landing.OwnerID != nil && landing.HandlingAutomationID == nil {
		owner := ids.From[ids.UserKind](*landing.OwnerID)
		in.OwnerID = &owner
	}
	in.ProductID = landing.ProductID
	// A number without its country prefix is kept on the note rather than
	// refusing the visitor's whole submission over it.
	if _, err := values.ParsePhone(e.Phone); err == nil {
		in.Phone = &e.Phone
	}
	return in
}

// webFormCustomFields is each cf_<key> field that fits an active lead custom
// field, in the value the lead writer takes. One that names no field or does
// not fit is left to the note, so the visitor is never refused over it.
//
//craft:ignore naked-any the values are the lead writer's CustomFields, whose shape depends on the column type
func webFormCustomFields(active []fieldcatalog.Column, e webforms.Enquiry, fieldValue FieldValue) map[string]any {
	byName := make(map[string]fieldcatalog.Column, len(active))
	for _, col := range active {
		byName[col.Name] = col
	}
	sent := map[string][]string{}
	for _, f := range e.Extra {
		name := strings.ToLower(f.Name)
		if _, known := byName[name]; known {
			sent[name] = append(sent[name], f.Value)
		}
	}
	out := make(map[string]any, len(sent))
	for name, texts := range sent {
		// A checkbox group sends one field per ticked box; the import reads a
		// multiselect as one semicolon list.
		if value, fits := fieldValue(name, byName[name], strings.Join(texts, ";")); fits {
			out[name] = value
		}
	}
	return out
}

func webFormValidatedPicklists(active []fieldcatalog.Column, in contacts.CreateLeadInput) []string {
	var names []string
	for _, column := range active {
		if column.Type == fieldcatalog.TypePicklist {
			if _, valid := in.CustomFields[column.Name]; valid {
				names = append(names, column.Name)
			}
		}
	}
	return names
}

// originDetail is the page and campaign a lead arrived from, in one line.
func originDetail(e webforms.Enquiry) string {
	parts := make([]string, 0, len(e.UTM)+1)
	if e.PageURL != "" {
		parts = append(parts, e.PageURL)
	}
	for _, u := range e.UTM {
		parts = append(parts, u.Name+"="+u.Value)
	}
	return textcut.Runes(strings.Join(parts, " · "), maxOriginDetail)
}

// maxOriginDetail is the lead.origin_detail bound (lead_origin_detail_bounded).
const maxOriginDetail = 500

// webFormNote is the timeline note: the consent the form recorded, then what
// the visitor sent as a quote. Every line the visitor wrote is quoted, so no
// field of theirs can pass for a line Margince wrote, such as the consent.
func webFormNote(landing webforms.Landing) string {
	e := landing.Enquiry
	var b strings.Builder
	if e.Message != "" {
		b.WriteString(e.Message + "\n\n")
	}
	line := func(name, value string) {
		if value != "" {
			b.WriteString(name + ": " + value + "\n")
		}
	}
	line("Name", e.FullName)
	line("Email", e.Email)
	line("Phone", e.Phone)
	line("Company", e.Company)
	line("Title", e.Title)
	for _, f := range e.Extra {
		line(f.Name, f.Value)
	}
	line("Page", e.PageURL)
	line("Referrer", e.Referrer)
	for _, u := range e.UTM {
		line(u.Name, u.Value)
	}
	sent := quoted(strings.TrimSpace(b.String()))
	consent := webFormConsent(landing)
	if consent == "" || sent == "" {
		return consent + sent
	}
	return consent + "\n\n" + sent
}

// webFormRaw keeps the submission field by field on the note, where the
// activity read answers it as `web_form`. The body says the same as text.
//
//craft:ignore naked-any raw is the activity's free-form source payload column
func webFormRaw(landing webforms.Landing, validated []string) (map[string]any, error) {
	e := landing.Enquiry
	sent := crmcontracts.WebFormSubmission{
		Message: optional(e.Message), FullName: optional(e.FullName), Email: e.Email,
		Phone: optional(e.Phone), Company: optional(e.Company), Title: optional(e.Title),
		Extra: webFormValues(e.Extra), PageUrl: optional(e.PageURL), Referrer: optional(e.Referrer),
		Campaign: webFormValues(e.UTM),
	}
	if len(validated) != 0 {
		sent.ValidatedPicklists = &validated
	}
	if landing.HandlingAutomationID != nil {
		sent.HandlingAutomationId = new(openapi_types.UUID(*landing.HandlingAutomationID))
	}
	if landing.ConsentWording != nil {
		formID := openapi_types.UUID(landing.FormID)
		sent.Consent = &crmcontracts.WebFormConsent{
			Ticked: e.Consent, Wording: *landing.ConsentWording, PolicyVersion: landing.ConsentPolicyVersion,
			FormId: &formID, FormName: optional(landing.FormName), PageUrl: optional(e.PageURL),
		}
	}
	encoded, err := json.Marshal(sent)
	if err != nil {
		return nil, fmt.Errorf("compose: encoding the website form's fields: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil, fmt.Errorf("compose: decoding the website form's fields: %w", err)
	}
	return raw, nil
}

func webFormValues(fields []webforms.Field) []crmcontracts.WebFormValue {
	out := make([]crmcontracts.WebFormValue, len(fields))
	for i, f := range fields {
		out[i] = crmcontracts.WebFormValue{Name: f.Name, Value: f.Value}
	}
	return out
}

// webFormConsent is the form's own record of its consent box. The admin's
// wording is kept to one line, so the record is the one line it reads as.
func webFormConsent(landing webforms.Landing) string {
	switch {
	case landing.ConsentWording == nil:
		return ""
	case !landing.Enquiry.Consent:
		return systemtext.Literal("form.consent_no")
	}
	wording := mailcopy.OneLine(*landing.ConsentWording)
	if landing.ConsentPolicyVersion != nil {
		return fmt.Sprintf(systemtext.Copy("form.consent_policy", textlang.English), wording, mailcopy.OneLine(*landing.ConsentPolicyVersion))
	}
	return fmt.Sprintf(systemtext.Copy("form.consent_yes", textlang.English), wording)
}

func webFormBodyComponent(landing webforms.Landing, body string) *systemtext.Component {
	consent := webFormConsent(landing)
	if consent == "" {
		return systemtext.Prose(systemtext.Quote, "")
	}
	component := systemtext.Sentence("form.consent_no")
	if landing.Enquiry.Consent {
		before, _, _ := strings.Cut(systemtext.Copy("form.consent_yes", textlang.English), "%s")
		prefix := utf8.RuneCountInString(before)
		wording := utf8.RuneCountInString(mailcopy.OneLine(*landing.ConsentWording))
		component = systemtext.Sentence("form.consent_yes", systemtext.Excerpt(prefix, wording))
		if landing.ConsentPolicyVersion != nil {
			_, after, _ := strings.Cut(systemtext.Copy("form.consent_policy", textlang.English), "%s")
			between, _, _ := strings.Cut(after, "%s")
			component = systemtext.Sentence("form.consent_policy", systemtext.Excerpt(prefix, wording), systemtext.Excerpt(prefix+wording+utf8.RuneCountInString(between), utf8.RuneCountInString(mailcopy.OneLine(*landing.ConsentPolicyVersion))))
		}
	}
	if body != consent {
		offset := utf8.RuneCountInString(consent) + 2
		component = systemtext.Sequence(component, systemtext.Sentence("core.source", systemtext.Excerpt(offset, utf8.RuneCountInString(body)-offset)))
		component.Layout = "paragraphs"
	}
	return component
}

// quoted marks every line of text as a Markdown quote. It splits on each break
// a reader may draw, so no line of the text is shown unmarked.
func quoted(text string) string {
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var lines []string
	start := 0
	for i, r := range text {
		if mailcopy.LineBreak(r) {
			lines = append(lines, text[start:i])
			start = i + utf8.RuneLen(r)
		}
	}
	lines = append(lines, text[start:])
	for i, l := range lines {
		lines[i] = strings.TrimRight("> "+l, " ")
	}
	return strings.Join(lines, "\n")
}

func (l Lander) legacyOwnsInquiry(ctx context.Context, id ids.UUID, key string) (bool, error) {
	lead, err := l.leads.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
	if err != nil {
		return false, err
	}
	if lead.SourceSystem == nil || *lead.SourceSystem != provenance.WebFormSource || lead.SourceId == nil || *lead.SourceId != key {
		return false, nil
	}
	if l.legacy == nil {
		return false, errors.New("compose: legacy inquiry ownership reader is not configured")
	}
	return l.legacy.LegacyLeadOccurrenceOwned(ctx, id)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
