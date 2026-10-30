// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// webFormLeadSource is the lead and note `source` a website form stamps.
const webFormLeadSource = "web_form"

// webFormLeads lands a website form submission through the lead and activity
// writers. A new email is a new lead. An email an open lead holds gets the
// note on that lead. An email a contact holds gets it on the contact, whose
// owner the lead writer has already told.
type webFormLeads struct {
	leads      *contacts.Store
	activities *activities.Store
	catalog    fieldcatalog.Reader
}

func (l webFormLeads) Land(ctx context.Context, landing webforms.Landing) error {
	target, err := l.lead(ctx, landing)
	if err != nil || target == nil {
		return err
	}
	system, key := provenance.WebFormSource, landing.SourceKey
	subject := "Website form: " + landing.FormName
	body := webFormNote(landing)
	raw, err := webFormRaw(landing)
	if err != nil {
		return err
	}
	_, _, err = l.activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Links: []activities.ActivityLinkInput{*target},
		SourceSystem: &system, SourceID: &key, Source: webFormLeadSource, Raw: &raw,
	})
	if err != nil {
		return fmt.Errorf("compose: writing the website form's note: %w", err)
	}
	return nil
}

// lead answers the record the note belongs on. Nil with no error is a contact
// this principal may not read. Its owner has a task about it, and nothing more
// is written.
func (l webFormLeads) lead(ctx context.Context, landing webforms.Landing) (*activities.ActivityLinkInput, error) {
	active, err := l.catalog.ActiveColumns(ctx, "lead")
	if err != nil {
		return nil, fmt.Errorf("compose: reading the lead custom fields: %w", err)
	}
	in := webFormLeadInput(landing)
	in.CustomFields = webFormCustomFields(active, landing.Enquiry)
	lead, _, err := l.leads.CreateLead(ctx, in)
	if _, fieldGone := errors.AsType[storekit.CustomFieldErrors](err); fieldGone {
		// A field was retired or changed since the catalog read. The values
		// are on the note, so the lead lands without them.
		in.CustomFields = nil
		lead, _, err = l.leads.CreateLead(ctx, in)
	}
	if _, ownerGone := errors.AsType[*auth.AssigneeNotAllowedError](err); ownerGone {
		// The form's owner was deactivated after it was saved. The lead goes to
		// the unassigned queue rather than being refused to the visitor.
		in.OwnerID = nil
		lead, _, err = l.leads.CreateLead(ctx, in)
	}
	if duplicate, ok := errors.AsType[*contacts.DuplicateLeadError](err); ok {
		return &activities.ActivityLinkInput{EntityType: string(recordTypeLead), EntityID: duplicate.ExistingID.UUID}, nil
	}
	if held, ok := errors.AsType[*contacts.ContactHoldsIdentityError](err); ok {
		if held.ContactID.IsZero() {
			return nil, nil //nolint:nilnil // a contact this principal may not read has no note target, and that is no failure
		}
		return &activities.ActivityLinkInput{EntityType: string(recordTypeContact), EntityID: held.ContactID.UUID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &activities.ActivityLinkInput{EntityType: string(recordTypeLead), EntityID: ids.UUID(lead.Id)}, nil
}

func webFormLeadInput(landing webforms.Landing) contacts.CreateLeadInput {
	e := landing.Enquiry
	system, key := provenance.WebFormSource, landing.SourceKey
	in := contacts.CreateLeadInput{
		Email: &e.Email, FullName: optional(e.FullName), Title: optional(e.Title),
		CompanyName: optional(e.Company), SourceSystem: &system, SourceID: &key,
		Source: webFormLeadSource, Origin: landing.LeadOrigin, OriginDetail: optional(originDetail(e)),
	}
	if landing.OwnerID != nil {
		owner := ids.From[ids.UserKind](*landing.OwnerID)
		in.OwnerID = &owner
	}
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
func webFormCustomFields(active []fieldcatalog.Column, e webforms.Enquiry) map[string]any {
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
		// A checkbox group sends one field per ticked box; checkCell reads a
		// multiselect as one semicolon list.
		if value, problem := checkCell(name, byName[name], strings.Join(texts, ";"), false); problem == nil {
			out[name] = value
		}
	}
	return out
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
func webFormRaw(landing webforms.Landing) (map[string]any, error) {
	e := landing.Enquiry
	sent := crmcontracts.WebFormSubmission{
		Message: optional(e.Message), FullName: optional(e.FullName), Email: e.Email,
		Phone: optional(e.Phone), Company: optional(e.Company), Title: optional(e.Title),
		Extra: webFormValues(e.Extra), PageUrl: optional(e.PageURL), Referrer: optional(e.Referrer),
		Campaign: webFormValues(e.UTM),
	}
	if landing.ConsentWording != nil {
		sent.Consent = &crmcontracts.WebFormConsent{
			Ticked: e.Consent, Wording: *landing.ConsentWording, PolicyVersion: landing.ConsentPolicyVersion,
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
		return "Consent: not ticked"
	}
	consent := "Consent: ticked: " + mailcopy.OneLine(*landing.ConsentWording)
	if landing.ConsentPolicyVersion != nil {
		consent += " (policy " + mailcopy.OneLine(*landing.ConsentPolicyVersion) + ")"
	}
	return consent
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
