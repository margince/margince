// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var slippingWords = struct {
	Idle, Overdue, NoNextStep langcopy.Phrase
}{
	Idle:       langcopy.Phrase{En: "no recorded activity since %s", De: "keine erfasste Aktivität seit %s", Vi: "không có hoạt động được ghi nhận kể từ %s"},
	Overdue:    langcopy.Phrase{En: "expected close %s is past due", De: "der erwartete Abschluss am %s ist überfällig", Vi: "ngày chốt dự kiến %s đã quá hạn"},
	NoNextStep: langcopy.Phrase{En: "no open next step on this deal", De: "kein offener nächster Schritt für diesen Deal", Vi: "không có bước tiếp theo đang mở cho giao dịch này"},
}
