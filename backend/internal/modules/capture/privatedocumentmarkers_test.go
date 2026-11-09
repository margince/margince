// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

func TestACVIsTitledOrBuiltFromCareerHeadings(t *testing.T) {
	for name, text := range map[string]string{
		"a German title":   "Max Mustermann\nLebenslauf\nMünchen",
		"an English title": "CURRICULUM VITAE\nJane Doe",
		"experience and education": `Jane Doe
Senior PHP Developer
WORK EXPERIENCE
2020 – today Backend developer at Northwind
EDUCATION:
2014 – 2018 B.Sc. Informatics`,
		"German sections with bullets": "• Berufserfahrung\n2018 Entwickler\n• Sprachkenntnisse\nDeutsch, Englisch",
		"skills and a date of birth":   "Skills\nGo, SQL\nDate of birth: 01.02.1990",
		"a summary and experience":     "Jane Doe\nSummary\nTen years of PHP.\nExperience\nHead of Development",
		"letter-spaced headings":       "ABOU T M E\nI build shops.\nEDU C ATIO N\nUniversity",
		"a letter-spaced title":        "L E B E N S L A U F\nMax Mustermann",
		"a bulleted title":             "• CV •\nJane Doe",
		"a title merged into a line":   "Curriculum Vitae Anna Berger Work Experience Education Skills Languages",
	} {
		if !DescribesACareer(text) {
			t.Errorf("%s: not read as a CV", name)
		}
	}
}

func TestOrdinaryDocumentsAreNotCVs(t *testing.T) {
	for name, text := range map[string]string{
		"a proposal with one experience heading": "Proposal\nExperience\nWe have built ten shops.\nPricing\n12,000.00",
		"prose that starts with a heading word":  "Experience shows that most projects slip, and education of the team helps.",
		"a job advert":                           "Senior Developer (m/w/d)\nSkills\nGo, SQL\nWhat we offer",
		"a word that only starts like a heading": "Experienced team\nEducational content",
		"a proposal with a summary and a price":  "Executive Summary\nWe propose a shop.\nSummary\nPricing\n12,000.00",
		"a list of folders with a CV folder":     "1.1 Contracts\n1.2 CV\n1.3 Invoices",
	} {
		if DescribesACareer(text) {
			t.Errorf("%s: read as a CV", name)
		}
	}
}

func TestAFileNamedAsAnInvoiceCostSheetOrCVIsNamedPrivate(t *testing.T) {
	for _, name := range []string{
		"2023.09 ROADSURFER INV 3556.pdf", "2023.11 Northwind Inv 3667.pdf", "Rechnung_2024-03.pdf",
		"2026 11 Northwind Cost+ VER 2.xlsx", "CV_Jane_Doe.pdf", "Lebenslauf Max.pdf", "INV3556.pdf",
		"PhamHoangThucCV.pdf", "Résumé - Jane Doe.pdf", "invo\u200bice.PDF", "re\u0301sume\u0301.pdf",
		"salaryinvoice.pdf", "passport_scan.png", "Reisepass.pdf", "ID_card.jpg", "visa.pdf",
		"Befund.pdf", "COVID19.pdf", "test_result.pdf", "medical_test_results.pdf", "personalausweis.pdf",
	} {
		if !NamedAsPrivate(name) {
			t.Errorf("%q: not named private", name)
		}
	}
	for _, name := range []string{"venue map.pdf", "Invitation.pdf", "Termsheet.pdf", "scan0001.pdf", "Recovery plan.pdf", "project_ID.pdf", "card_layout.pdf", "contest_results.pdf", "advisable.pdf"} {
		if NamedAsPrivate(name) {
			t.Errorf("%q: named private", name)
		}
	}
}

func TestOnlyACVNameHoldsAFileTheVerdictCanRead(t *testing.T) {
	for _, name := range []string{"renefischer_cv2025.pdf", "Resume_FE_Mid_Jane.pdf", "PhamHoangThucCV.pdf"} {
		if !NamedAsCV(name) {
			t.Errorf("%q: not named a CV", name)
		}
	}
	for _, name := range []string{"03.2024 Detailed Invoice.xlsx", "Rechnung 1179.pdf", "Cost+ Agreement.pdf", "CVS receipt.pdf"} {
		if NamedAsCV(name) {
			t.Errorf("%q: named a CV", name)
		}
	}
}
