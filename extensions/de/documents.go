// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package de

// The compliance documents an installation under German law signs before it
// reads its own team's mail. The German originals bind; the English copies are
// for reading. The Jurisdiction settings page lists and serves them.

import (
	"embed"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension/compliancedocs"
)

//go:embed documents/de/*.md documents/en/*.md
var documentFiles embed.FS

// documentEntry names one embedded file and how the page lists it.
type documentEntry struct {
	path     string
	id       string
	title    string
	language string
	audience compliancedocs.Audience
	kind     compliancedocs.Kind
}

var documentEntries = []documentEntry{
	{"documents/de/mitarbeiterinformation.md", "mitarbeiterinformation", "Mitarbeiterinformation zur Erfassung von E-Mail-Korrespondenz", "de", compliancedocs.Employee, compliancedocs.Template},
	{"documents/de/empfangsbestaetigung.md", "empfangsbestaetigung", "Empfangsbestätigung", "de", compliancedocs.Employee, compliancedocs.Template},
	{"documents/de/einwilligung-email-erfassung.md", "einwilligung-email-erfassung", "Einwilligung in die Erfassung privater E-Mail-Korrespondenz", "de", compliancedocs.Employee, compliancedocs.Template},
	{"documents/de/betriebsvereinbarung-vorlage.md", "betriebsvereinbarung-vorlage", "Betriebsvereinbarung über die Erfassung von E-Mail-Korrespondenz", "de", compliancedocs.Employer, compliancedocs.Template},
	{"documents/de/verarbeitungsverzeichnis-und-dsfa.md", "verarbeitungsverzeichnis-und-dsfa", "Verzeichnis von Verarbeitungstätigkeiten und Datenschutz-Folgenabschätzung", "de", compliancedocs.Operator, compliancedocs.Record},
	{"documents/en/employee-information.md", "employee-information", "Information for employees: how your mail is captured", "en", compliancedocs.Employee, compliancedocs.ReadingCopy},
	{"documents/en/acknowledgement-of-receipt.md", "acknowledgement-of-receipt", "Acknowledgement of receipt", "en", compliancedocs.Employee, compliancedocs.ReadingCopy},
	{"documents/en/consent-private-mail.md", "consent-private-mail", "Consent to capturing private correspondence", "en", compliancedocs.Employee, compliancedocs.ReadingCopy},
	{"documents/en/works-agreement.md", "works-agreement", "Works agreement on mail capture", "en", compliancedocs.Employer, compliancedocs.ReadingCopy},
	{"documents/en/processing-record-and-dpia.md", "processing-record-and-dpia", "Processing record and impact assessment", "en", compliancedocs.Operator, compliancedocs.ReadingCopy},
}

// documents reads every listed file. A listed file that is not embedded is a
// build defect, so it panics at the first boot. A page must not list a
// document it cannot serve.
func documents() []compliancedocs.Document {
	out := make([]compliancedocs.Document, 0, len(documentEntries))
	for _, entry := range documentEntries {
		body, err := documentFiles.ReadFile(entry.path)
		if err != nil {
			panic(fmt.Sprintf("de: document %s is listed but not embedded: %v", entry.path, err))
		}
		out = append(out, compliancedocs.Document{
			Jurisdiction: "de", ID: entry.id, Title: entry.title, Language: entry.language,
			Audience: entry.audience, Kind: entry.kind, Body: body, MediaType: "text/markdown",
		})
	}
	return out
}
