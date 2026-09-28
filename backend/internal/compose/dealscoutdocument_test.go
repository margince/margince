// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

func TestAProposalIsNamedByAWordOfItsFileName(t *testing.T) {
	const pdf = "application/pdf"
	cases := []struct {
		filename, contentType string
		want                  bool
	}{
		{"Angebot_2026.pdf", pdf, true},
		{"Angebot2026.pdf", pdf, true},
		{"Kostenvoranschlag Müller GmbH.pdf", pdf, true},
		{"Proposal - Acme.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"SOW_v3.pdf", pdf, true},
		{"statement-of-work.pdf", pdf, true},
		{"Báo giá dự án.pdf", pdf, true},
		{"Hợp đồng dịch vụ.pdf", pdf, true},
		{"hop_dong.pdf", pdf, true},
		{"Quote.pdf", "", true},

		{"Newsletter_Angebote.pdf", pdf, false},
		{"Angebot_und_Rechnung.pdf", pdf, false},
		{"Invoice-Quote-123.pdf", pdf, false},
		{"Hóa đơn báo giá.pdf", pdf, false},
		{"Vertragsnummer.pdf", pdf, false},
		{"Angebot.png", "image/png", false},
		{"Angebot.pdf", "image/png", false},
		{"Angebot.exe", "", false},
		{"Quotes.pdf", pdf, false},
		{"Agenda.pdf", pdf, false},
	}
	for _, c := range cases {
		if got := isProposalDocument(c.filename, c.contentType); got != c.want {
			t.Errorf("isProposalDocument(%q, %q) = %v, want %v", c.filename, c.contentType, got, c.want)
		}
	}
}

func TestAFileNameSplitsIntoFoldedWords(t *testing.T) {
	got := filenameWords("Báo_giá-Angebot2026 v2")
	want := []string{"bao", "gia", "angebot", "2026", "v", "2"}
	if len(got) != len(want) {
		t.Fatalf("filenameWords = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filenameWords = %q, want %q", got, want)
		}
	}
}
