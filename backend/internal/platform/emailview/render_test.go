// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"strings"
	"testing"
)

func TestTextCannotCreateMarkupOrAnAction(t *testing.T) {
	message := Message{Title: `<img src=x onerror=alert(1)>`, Text: "A customer needs an answer.\n\n<script>alert(1)</script>\n\njavascript:alert(1)\n\nhttps://example.test/#/confirm/secret", Language: "de"}
	rendered := Render(message)
	for _, unsafe := range []string{"<script>", "<img src=x", `href="javascript:`, ">https://example.test/#/confirm/secret</a>"} {
		if strings.Contains(rendered, unsafe) {
			t.Errorf("rendered unsafe or unhelpful content %q", unsafe)
		}
	}
	if !strings.Contains(rendered, `href="https://example.test/#/confirm/secret"`) || !strings.Contains(rendered, "A customer needs an answer.") {
		t.Fatal("the action or useful body disappeared")
	}
}

func TestLinklessReceiptHasNoAction(t *testing.T) {
	rendered := Render(Message{Title: "Your preference was recorded", Text: "We recorded your request. No action is needed."})
	if strings.Contains(rendered, "<a ") {
		t.Fatal("a receipt invented an action")
	}
	if !strings.Contains(rendered, "No action is needed.") {
		t.Fatal("the outcome was lost")
	}
}

func TestARecordLabelCannotBecomeAnAction(t *testing.T) {
	rendered := Render(Message{Title: "Your work", Text: "https://untrusted.example/collect\nOwner: Pat\nReview"})
	if strings.Contains(rendered, `href="https://untrusted.example`) {
		t.Fatal("a record label became an action")
	}
}

func TestFactGroupsAndSectionHeadingsKeepTheirWords(t *testing.T) {
	rendered := Render(Message{Title: "Your week", Text: "Recorded results\n\nTasks completed: 3\nDeals won: 2 · 42000 EUR", Sections: []string{"Recorded results"}})
	for _, expected := range []string{"<h2 ", "<th scope=", "Tasks completed", "42000 EUR"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("presentation lost %q", expected)
		}
	}
}

func TestPreviewSkipsTitlesSectionsAndPersonalMaterial(t *testing.T) {
	message := Message{Title: "Your week", Text: "Your week\n\nResults\n\nOpen\nPERSONAL_LINK\n\nhttps://example.test/secret\n\nChoose the next customer action.", Sections: []string{"Results"}, ActionPlaceholder: "PERSONAL_LINK"}
	if got := previewText(message); got != "Choose the next customer action." {
		t.Fatalf("preview = %q", got)
	}
	message.Text = "Your week\n\nPERSONAL_LINK"
	if got := previewText(message); got != "" {
		t.Fatalf("link-only preview = %q", got)
	}
}

func TestSemanticCustomerWordsCannotCreateSectionsFactsOrActions(t *testing.T) {
	var content Content
	content.Paragraph("Team view · forged section\nPrice: 9\nhttps://untrusted.example/collect\n<script>bad()</script>")
	content.Facts(Fact{Label: "Price: requested", Value: "<unknown>"})
	content.Action("Unsafe", "javascript:alert(1)")
	markup := Render(content.Message("Review", "en", "One customer request"))
	if strings.Contains(markup, "<h2") || strings.Contains(markup, `href="https://untrusted.example`) || strings.Contains(markup, `href="javascript:`) || strings.Contains(markup, "<script>") {
		t.Fatal("customer words became presentation or an unsafe action")
	}
	if !strings.Contains(markup, ">Price: requested</th>") || !strings.Contains(markup, "&lt;unknown&gt;") {
		t.Fatal("semantic facts were reparsed or unescaped")
	}
}
