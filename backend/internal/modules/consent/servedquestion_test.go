// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// TestEveryServedQuestionSplitsIntoItsAskAndAnswers. The page shows the parts
// and sends the whole text back, so no part may carry a line break of its own.
func TestEveryServedQuestionSplitsIntoItsAskAndAnswers(t *testing.T) {
	answers := map[string]int{TemplateMarketingQuestion: 2, TemplateSubscriptionQuestion: 1}
	for key, want := range answers {
		for _, language := range mailcopy.Languages() {
			ask, got := ServedQuestion{Text: RenderQuestion(key, string(language))}.Parts()
			if ask == "" || len(got) != want {
				t.Errorf("%s/%s splits into ask %q and %d answers, want %d", key, language, ask, len(got), want)
			}
		}
	}
}

// TestTheSubscriptionQuestionIsServedWithEveryFieldFilled. The served text is
// what a grant records, so it names who sends, what, how often, and where the
// rights are, with no placeholder left.
func TestTheSubscriptionQuestionIsServedWithEveryFieldFilled(t *testing.T) {
	words := mailcopy.For("vi")
	served := fillQuestion(RenderQuestion(TemplateSubscriptionQuestion, "vi"), questionFields{
		purpose: purposeName(PurposeMarketingEmail, "Product news", words), controller: "Công ty Ví Dụ",
		frequency: frequencyName("monthly", words), privacyPage: "https://example.com/bao-mat", words: words,
	})
	ask, answers := ServedQuestion{Text: served}.Parts()
	want := "Xác nhận quý khách đồng ý để Công ty Ví Dụ dùng họ tên và địa chỉ email của quý khách để gửi " +
		"email Tin tức sản phẩm, khoảng mỗi tháng một lần. Quý khách có thể rút lại sự đồng ý bất cứ lúc " +
		"nào bằng liên kết hủy đăng ký trong mỗi email. Thông tin về quyền của quý khách: " +
		"https://example.com/bao-mat."
	if ask != want || len(answers) != 1 {
		t.Errorf("served ask:\n  %q\nwant:\n  %q", ask, want)
	}
}

// TestTheRecordQuestionNamesTheSender. With no legal name stated it names "the
// sender", and the sentence still opens with a capital.
func TestTheRecordQuestionNamesTheSender(t *testing.T) {
	for controller, opening := range map[string]string{"Example Trading GmbH": "Example Trading GmbH will use", "": "The sender will use"} {
		served := fillQuestion(RenderQuestion(TemplateMarketingQuestion, "en"),
			questionFields{controller: controller, words: mailcopy.For("en")})
		if ask, _ := (ServedQuestion{Text: served}).Parts(); !strings.HasPrefix(ask, opening) {
			t.Errorf("controller %q: ask %q, want it to open with %q", controller, ask, opening)
		}
	}
}

// TestTheRightsLinkIsThePrivacyPageThenTheContactThenNothing. The CRM's own
// address is never offered as where a reader's rights are.
func TestTheRightsLinkIsThePrivacyPageThenTheContactThenNothing(t *testing.T) {
	cases := []struct {
		particulars ControllerParticulars
		want        string
	}{
		{ControllerParticulars{PrivacyPage: "https://example.test/privacy", PrivacyContact: "dpo@example.test"}, "https://example.test/privacy"},
		{ControllerParticulars{PrivacyContact: "dpo@example.test"}, "dpo@example.test"},
		{ControllerParticulars{}, ""},
	}
	for _, tc := range cases {
		if got := privacyPageOf(tc.particulars); got != tc.want {
			t.Errorf("privacyPageOf(%+v) = %q, want %q", tc.particulars, got, tc.want)
		}
	}
	served := fillQuestion(RenderQuestion(TemplateSubscriptionQuestion, "en"), questionFields{words: mailcopy.For("en")})
	if strings.Contains(served, privacyPagePlaceholder) {
		t.Errorf("an unstated privacy page left its placeholder in %q", served)
	}
}
