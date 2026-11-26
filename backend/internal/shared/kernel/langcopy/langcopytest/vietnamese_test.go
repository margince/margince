// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

func TestVietnameseTextHoldsTheMechanicalRules(t *testing.T) {
	t.Parallel()
	assertStyle(t, []styleCase{
		{"Đã xóa", ""},
		{"Đã xoá", ruleViToneOpen},
		{"Tuỳ chọn", ruleViToneOpen},
		{"Hoà hợp", ruleViToneOpen},
		{"Quỹ và quý", ""},
		{"Hòan tất", ruleViToneClosed},
		{"Hoàn tất và khuyên dùng", ""},
		{"Đã lưu — thử lại", ruleViDash},
		{"Đã lưu -- thử lại", ruleViDash},
		{"Đã lưu - thử lại", ruleViDash},
		{"Đã lưu, thử lại", ""},
		{"Đã lưu!", ruleViExclamation},
		{"Máy chủ", ""},
		{"Ma\u0301y chủ", ruleViCombiningMark},
		{"Máy chủ ☕\uFE0F", ""},
		{"Mục “Tài liệu”", ""},
		{`Mục "Tài liệu"`, ruleViQuotes},
		{"Mục „Tài liệu“", ruleViQuotes},
		{"Mục «Tài liệu»", ruleViQuotes},
		{"Lena's", ruleViQuotes},
		{"Lena’s", ""},
		{"Tôi đã lưu", ruleViToi},
		{"Chúng tôi không lưu được", ruleViChungToi},
		{"Chúng ta cần lưu", ruleViChungTa},
		{"Việc của mình", ruleViChungTa},
		{"Họ tự mình quyết định", ""},
		{"Kính thưa quý vị", ruleViFormal},
		{"Anh chị xem lại", ruleViFormal},
		{"Anh chị em của họ", ""},
		{"Lưu lại nhé", ruleViParticle},
		{"Cảm ơn ạ", ruleViParticle},
		{"Nhà của bạn", ""},
		{"Vui lòng thử lại", ruleViMemberAddress},
		{"Thêm thương vụ", ruleViRetired},
		{"Thêm Thương Vụ", ruleViRetired},
		{"Mở tệp report", ruleViRetired},
		{"Mở tệp report.pdf", ""},
		{"Hiệu suất của nhóm", ""},
		{"Mở “Thương vụ” trong sản phẩm kia", ""},
		{"Mở %[1]s cho {name}", ""},
	}, func(r reporter, text string) { VietnameseText(r, "Field", text) })
}

func TestEachRegisterHoldsItsOwnVietnameseVoice(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		register Register
		cases    []styleCase
	}{
		{Outsider("a buyer reads it"), []styleCase{
			{"Vui lòng xác nhận, quý khách", ""},
			{"Bạn hãy xác nhận", ruleViOutsiderAddress},
			{"Chúng tôi đã nhận", ruleViChungToi},
		}},
		{Controller("the data controller writes it"), []styleCase{
			{"Chúng tôi lưu thông tin của quý khách", ""},
			{"Tôi lưu thông tin", ruleViToi},
			{"Bạn có thể phản đối", ruleViOutsiderAddress},
		}},
		{Agent("an agent speaks in a chat bubble"), []styleCase{{"Tôi đã đọc trang web", ""}}},
		{OwnVoice("the member sends it as themselves"), []styleCase{
			{"Anh chị giới thiệu giúp mình nhé", ""}, {"Cảm ơn — hẹn gặp lại", ruleViDash},
		}},
		{Verbatim("a word list a matcher reads"), []styleCase{
			{"Kính gửi anh chị — xoá!", ""},
		}},
	} {
		assertStyle(t, c.cases, func(r reporter, text string) { c.register.VietnameseText(r, "Field", text) })
	}
}

// Census reads the English beside the Vietnamese, so tôi may stand for me.
func TestCensusLetsTôiTranslateAnEnglishMe(t *testing.T) {
	t.Parallel()
	type table struct{ Assign langcopy.Phrase }
	good := table{Assign: langcopy.Phrase{En: "Assign to me", De: "Mir zuweisen", Vi: "Giao cho tôi"}}
	if got := rulesReported(func(r reporter) { Census(r, good) }); len(got) != 0 {
		t.Errorf("tôi for an English me was reported for %v", got)
	}
	bad := table{Assign: langcopy.Phrase{En: "Assigned", De: "Zugewiesen", Vi: "Tôi đã giao"}}
	if got := rulesReported(func(r reporter) { Census(r, bad) }); strings.Join(got, " ") != ruleViToi {
		t.Errorf("tôi with no English me was reported for %v", got)
	}
}
