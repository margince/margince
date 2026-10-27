// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"sort"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// The lead fields a submission is read into, by the names a field_map
// targets and the snippet posts.
const (
	fieldEmail     = "email"
	fieldFullName  = "full_name"
	fieldFirstName = "first_name"
	fieldLastName  = "last_name"
	fieldPhone     = "phone"
	fieldCompany   = "company"
	fieldTitle     = "title"
	fieldMessage   = "message"
	fieldConsent   = "consent"
	fieldPageURL   = "page_url"
	fieldReferrer  = "referrer"
)

// leadFieldNames lists the targets a field_map may name.
var leadFieldNames = []string{
	fieldEmail, fieldFullName, fieldFirstName, fieldLastName, fieldPhone, fieldCompany,
	fieldTitle, fieldMessage, fieldConsent, fieldPageURL, fieldReferrer,
}

// aliasSpellings are the field names form plugins and builders commonly use
// for each target, folded: lower case, letters and digits only. So
// "Your-Email", "your_email" and "youremail" are one spelling.
var aliasSpellings = map[string]string{
	fieldEmail:     "email emailaddress youremail mail",
	fieldFullName:  "name fullname yourname",
	fieldFirstName: "firstname fname givenname vorname",
	fieldLastName:  "lastname lname surname familyname nachname",
	fieldPhone:     "phone phonenumber tel telephone mobile telefon",
	fieldCompany:   "company companyname organization organisation firma unternehmen",
	fieldTitle:     "title jobtitle position",
	fieldMessage:   "message yourmessage comments comment enquiry inquiry nachricht",
	fieldConsent:   "consent privacy gdpr datenschutz",
	fieldPageURL:   "pageurl",
	fieldReferrer:  "referrer referer",
}

// fieldAliases reads a folded field name as its target.
var fieldAliases = func() map[string]string {
	out := map[string]string{}
	for target, spellings := range aliasSpellings {
		for spelling := range strings.FieldsSeq(spellings) {
			out[spelling] = target
		}
	}
	return out
}()

// controlPrefix marks the fields the snippet adds for the spam checks. They
// are read for admission and never kept.
const controlPrefix = "_mrg_"

// The control fields.
const (
	controlHoneypot = controlPrefix + "hp"
	controlRendered = controlPrefix + "ts"
	controlID       = controlPrefix + "id"
)

// Bounds on what one submission may carry.
const (
	maxFields       = 100
	maxValueLength  = 2000
	maxMessageChars = 10000
)

// field is one submitted name and value, in the order the sender gave them.
type field struct {
	Name  string
	Value string
}

// Enquiry is a submission read into lead fields. Extra keeps every field the
// lead has no column for, so nothing the visitor wrote is dropped.
type Enquiry struct {
	Email     string
	FullName  string
	Phone     string
	Company   string
	Title     string
	Message   string
	Consent   bool
	PageURL   string
	Referrer  string
	UTM       []field
	Extra     []field
	controls  map[string]string
	firstName string
	lastName  string
}

func fold(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// readEnquiry maps submitted fields onto lead fields: the form's own field_map
// first, then the common spellings. The first value for a target wins.
func readEnquiry(fields []field, fieldMap map[string]string) Enquiry {
	mapped := make(map[string]string, len(fieldMap))
	for from, to := range fieldMap {
		mapped[fold(from)] = to
	}
	e := Enquiry{controls: map[string]string{}}
	seen := map[string]bool{}
	for i, f := range fields {
		if i >= maxFields {
			break
		}
		if strings.HasPrefix(f.Name, controlPrefix) {
			e.controls[f.Name] = f.Value
			continue
		}
		value := strings.TrimSpace(f.Value)
		if value == "" {
			continue
		}
		key := fold(f.Name)
		target, ok := mapped[key]
		if !ok {
			target, ok = fieldAliases[key]
		}
		switch {
		case ok && !seen[target]:
			seen[target] = true
			e.set(target, value)
		case strings.HasPrefix(strings.ToLower(f.Name), "utm_"):
			e.UTM = append(e.UTM, field{Name: strings.ToLower(f.Name), Value: textcut.Runes(value, maxValueLength)})
		default:
			e.Extra = append(e.Extra, field{Name: textcut.Runes(f.Name, maxFieldNameLength), Value: textcut.Runes(value, maxValueLength)})
		}
	}
	if e.FullName == "" {
		e.FullName = strings.TrimSpace(e.firstName + " " + e.lastName)
	}
	return e
}

func (e *Enquiry) set(target, value string) {
	switch target {
	case fieldEmail:
		e.Email = strings.ToLower(textcut.Runes(value, maxValueLength))
	case fieldFullName:
		e.FullName = textcut.Runes(value, maxValueLength)
	case fieldFirstName:
		e.firstName = textcut.Runes(value, maxValueLength)
	case fieldLastName:
		e.lastName = textcut.Runes(value, maxValueLength)
	case fieldPhone:
		e.Phone = textcut.Runes(value, maxValueLength)
	case fieldCompany:
		e.Company = textcut.Runes(value, maxValueLength)
	case fieldTitle:
		e.Title = textcut.Runes(value, maxValueLength)
	case fieldMessage:
		e.Message = textcut.Runes(value, maxMessageChars)
	case fieldConsent:
		e.Consent = ticked(value)
	case fieldPageURL:
		e.PageURL = textcut.Runes(value, maxValueLength)
	case fieldReferrer:
		e.Referrer = textcut.Runes(value, maxValueLength)
	}
}

// ticked reads a checkbox: browsers send "on", builders send booleans, and
// German forms say "ja".
func ticked(value string) bool {
	switch strings.ToLower(value) {
	case "on", "true", "1", "yes", "ja", "checked", "accepted":
		return true
	}
	return false
}

// plausibleEmail keeps an obviously broken address from the lead writer. That
// writer still normalises and refuses on its own.
func plausibleEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-3 && strings.Contains(email[at:], ".") &&
		!strings.ContainsAny(email, " \t\r\n<>,;") && len(email) <= 254
}

// sortedFields answers a map's entries by name, for senders whose payload
// carries no order of its own.
func sortedFields(m map[string]string) []field {
	out := make([]field, 0, len(m))
	for k, v := range m {
		out = append(out, field{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
