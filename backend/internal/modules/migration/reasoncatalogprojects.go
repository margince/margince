// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The reasons a project or product row is skipped: what the file left out or
// wrote in a way the stores refuse. They sit apart from the other objects'
// catalog to keep each file under the length cap, and withProjectReasons joins
// the two.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for project and product rows.
const (
	ReasonProjectUnnamed        ReasonCode = "project_unnamed"
	ReasonProjectCompanyMissing ReasonCode = "project_company_missing"
	ReasonProjectCompanyFixed   ReasonCode = "project_company_fixed"
	ReasonProjectPhaseUnknown   ReasonCode = "project_phase_unknown"
	ReasonProjectPhaseClosed    ReasonCode = "project_phase_closed"
	ReasonProjectDatesInverted  ReasonCode = "project_dates_inverted"
	ReasonProjectDealRefused    ReasonCode = "project_deal_refused"
	ReasonProductUnnamed        ReasonCode = "product_unnamed"
	ReasonProductSKUHeld        ReasonCode = "product_sku_held"
)

// withProjectReasons adds the project and product reasons to the catalog the
// other objects declare, and answers it.
func withProjectReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, projectReasons)
	return catalog
}

var projectReasons = map[ReasonCode]reasonEntry{
	ReasonProjectUnnamed: {text: map[textlang.Lang]string{
		textlang.English:    "Give the project a name.",
		textlang.German:     "Geben Sie dem Projekt einen Namen.",
		textlang.Vietnamese: "Hãy đặt tên cho dự án.",
	}},
	ReasonProjectCompanyMissing: {text: map[textlang.Lang]string{
		textlang.English:    "A project belongs to a company. Name it in the company column.",
		textlang.German:     "Ein Projekt gehört zu einem Unternehmen. Nennen Sie es in der Unternehmensspalte.",
		textlang.Vietnamese: "Dự án thuộc về một công ty. Hãy nêu công ty trong cột công ty.",
	}},
	ReasonProjectCompanyFixed: {text: map[textlang.Lang]string{
		textlang.English:    "A project keeps the company it was created for, so this row is not applied. Leave the company column as it is, or import the row as a new project.",
		textlang.German:     "Ein Projekt behält das Unternehmen, für das es erstellt wurde, daher wird diese Zeile nicht übernommen. Lassen Sie die Unternehmensspalte unverändert oder importieren Sie die Zeile als neues Projekt.",
		textlang.Vietnamese: "Dự án giữ công ty mà nó được tạo cho, nên hàng này không được áp dụng. Hãy giữ nguyên cột công ty, hoặc nhập hàng này làm dự án mới.",
	}},
	ReasonProjectPhaseUnknown: {params: []string{paramValue, "names"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a phase a project starts in. The phases are: {names}.",
		textlang.German:     "{value:q} ist keine Startphase eines Projekts. Mögliche Phasen: {names}.",
		textlang.Vietnamese: "{value:q} không phải giai đoạn bắt đầu của dự án. Các giai đoạn: {names}.",
	}},
	ReasonProjectPhaseClosed: {text: map[textlang.Lang]string{
		textlang.English:    "A project is not created closed, because closing needs a reason. Import it in another phase, then close it in the app.",
		textlang.German:     "Ein Projekt wird nicht geschlossen erstellt, weil das Schließen einen Grund braucht. Importieren Sie es in einer anderen Phase und schließen Sie es dann in der App.",
		textlang.Vietnamese: "Dự án không được tạo ở trạng thái đã đóng, vì đóng dự án cần có lý do. Hãy nhập ở giai đoạn khác rồi đóng trong ứng dụng.",
	}},
	ReasonProjectDatesInverted: {text: map[textlang.Lang]string{
		textlang.English:    "The target end date is before the start date.",
		textlang.German:     "Das geplante Enddatum liegt vor dem Startdatum.",
		textlang.Vietnamese: "Ngày kết thúc dự kiến nằm trước ngày bắt đầu.",
	}},
	ReasonProjectDealRefused: {params: []string{"detail"}, text: map[textlang.Lang]string{
		textlang.English:    "The deal cannot start this project: {detail}",
		textlang.German:     "Der Deal kann dieses Projekt nicht starten: {detail}",
		textlang.Vietnamese: "Deal không thể khởi động dự án này: {detail}",
	}},
	ReasonProductUnnamed: {text: map[textlang.Lang]string{
		textlang.English:    "Give the product a name.",
		textlang.German:     "Geben Sie dem Produkt einen Namen.",
		textlang.Vietnamese: "Hãy đặt tên cho sản phẩm.",
	}},
	ReasonProductSKUHeld: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "Another live product already has the SKU “{value}”.",
		textlang.German:     "Ein anderes aktives Produkt hat bereits die Artikelnummer {value:q}.",
		textlang.Vietnamese: "Một sản phẩm đang dùng khác đã có mã SKU {value:q}.",
	}},
}
