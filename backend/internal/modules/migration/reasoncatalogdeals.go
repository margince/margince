// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The reasons a deal row is skipped or warned about: its pipeline, stage, money,
// close date and links. They sit apart from the other objects' catalog to keep
// each file under the length cap, and withDealReasons joins the two.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for deal rows, each with its reasons text in the catalog below.
const (
	ReasonCannotReadPipelines    ReasonCode = "cannot_read_pipelines"
	ReasonStageUnnamed           ReasonCode = "stage_unnamed"
	ReasonPipelineUnnamed        ReasonCode = "pipeline_unnamed"
	ReasonPipelineUnknown        ReasonCode = "pipeline_unknown"
	ReasonStageUnknown           ReasonCode = "stage_unknown"
	ReasonCurrencyNotACode       ReasonCode = "currency_not_a_code"
	ReasonAmountNegative         ReasonCode = "amount_negative"
	ReasonCloseDatePast          ReasonCode = "close_date_past"
	ReasonCloseDateFuture        ReasonCode = "close_date_future"
	ReasonLostReasonMissing      ReasonCode = "lost_reason_missing"
	ReasonDealPipelineFixed      ReasonCode = "deal_pipeline_fixed"
	ReasonDealAlreadyClosed      ReasonCode = "deal_already_closed"
	ReasonFxRateMissing          ReasonCode = "fx_rate_missing"
	ReasonDealLegalGate          ReasonCode = "deal_legal_gate"
	ReasonPipelineNoOpenStage    ReasonCode = "pipeline_no_open_stage"
	ReasonCannotLinkStakeholders ReasonCode = "cannot_link_stakeholders"
	ReasonContactNotFound        ReasonCode = "contact_not_found"
	ReasonContactNotFoundDropped ReasonCode = "contact_not_found_dropped"
)

// withDealReasons adds the deal reasons to the catalog the other objects
// declare, and answers it.
func withDealReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, dealReasons)
	return catalog
}

var dealReasons = map[ReasonCode]reasonEntry{
	ReasonCannotReadPipelines: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot read pipelines, so a deal row naming its stage is skipped.",
		textlang.German:     "Sie dürfen keine Pipelines lesen, daher wird eine Zeile übersprungen, die eine Phase nennt.",
		textlang.Vietnamese: "Bạn không có quyền xem quy trình bán hàng nên hàng deal nêu giai đoạn bị bỏ qua.",
	}},
	ReasonStageUnnamed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Name the deal’s stage.",
		textlang.German:     "Nennen Sie die Phase des Deals.",
		textlang.Vietnamese: "Hãy nêu giai đoạn của deal.",
	}},
	ReasonPipelineUnnamed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "Name the pipeline: this CRM has no default pipeline.",
		textlang.German:     "Nennen Sie die Pipeline: Dieses CRM hat keine Standard-Pipeline.",
		textlang.Vietnamese: "Hãy nêu quy trình bán hàng: CRM này không có quy trình mặc định.",
	}},
	ReasonPipelineUnknown: {params: []string{paramValue, "names"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a pipeline. The pipelines are: {names}.",
		textlang.German:     "{value:q} ist keine Pipeline. Es gibt: {names}.",
		textlang.Vietnamese: "{value:q} không phải quy trình bán hàng. Các quy trình hiện có: {names}.",
	}},
	ReasonStageUnknown: {params: []string{paramValue, "pipeline", "names"}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a stage of pipeline “{pipeline}”. Its stages are: {names}.",
		textlang.German:     "{value:q} ist keine Phase der Pipeline {pipeline:q}. Mögliche Phasen: {names}.",
		textlang.Vietnamese: "{value:q} không phải giai đoạn của quy trình {pipeline:q}. Các giai đoạn: {names}.",
	}},
	ReasonCurrencyNotACode: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a currency code. Write its three letters, like EUR.",
		textlang.German:     "{value:q} ist kein Währungscode. Schreiben Sie die drei Buchstaben, etwa EUR.",
		textlang.Vietnamese: "{value:q} không phải mã tiền tệ. Hãy viết ba chữ cái, ví dụ EUR.",
	}},
	ReasonAmountNegative: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is below zero, and a deal’s amount cannot be.",
		textlang.German:     "{value:q} liegt unter null, und der Betrag eines Deals darf das nicht sein.",
		textlang.Vietnamese: "{value:q} nhỏ hơn 0, mà số tiền của deal thì không được âm.",
	}},
	ReasonCloseDatePast: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is in the past, and an open deal’s close date cannot be.",
		textlang.German:     "{value:q} liegt in der Vergangenheit, und das Abschlussdatum eines offenen Deals darf das nicht sein.",
		textlang.Vietnamese: "{value:q} đã qua, mà ngày chốt của deal đang mở thì không được ở quá khứ.",
	}},
	ReasonCloseDateFuture: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is in the future, and a closed deal’s close date cannot be.",
		textlang.German:     "{value:q} liegt in der Zukunft, und das Abschlussdatum eines abgeschlossenen Deals darf das nicht sein.",
		textlang.Vietnamese: "{value:q} ở tương lai, mà ngày chốt của deal đã đóng không được ở tương lai.",
	}},
	ReasonLostReasonMissing: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A lost deal needs the reason it was lost.",
		textlang.German:     "Ein verlorener Deal braucht den Grund, warum er verloren ging.",
		textlang.Vietnamese: "Deal đã thua cần có lý do thua.",
	}},
	ReasonDealPipelineFixed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This deal is in another pipeline than the file names. An import does not move a deal between pipelines, so the row is skipped.",
		textlang.German:     "Dieser Deal liegt in einer anderen Pipeline als in der Datei angegeben. Ein Import verschiebt keinen Deal zwischen Pipelines, daher wird die Zeile übersprungen.",
		textlang.Vietnamese: "Deal này nằm trong quy trình khác với quy trình trong tệp. Nhập không chuyển deal giữa các quy trình nên hàng bị bỏ qua.",
	}},
	ReasonDealAlreadyClosed: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "This deal is already closed, so the row is skipped. An import does not reopen a closed deal.",
		textlang.German:     "Dieser Deal ist bereits abgeschlossen, daher wird die Zeile übersprungen. Ein Import öffnet keinen abgeschlossenen Deal wieder.",
		textlang.Vietnamese: "Deal này đã đóng nên hàng bị bỏ qua. Nhập không mở lại deal đã đóng.",
	}},
	ReasonFxRateMissing: {params: []string{"currency"}, text: map[textlang.Lang]string{
		textlang.English:    "No exchange rate for {currency} is on file for the day this deal closed, so the row is skipped. Add the rate, then import again.",
		textlang.German:     "Für {currency} liegt kein Wechselkurs für den Abschlusstag dieses Deals vor, daher wird die Zeile übersprungen. Hinterlege den Kurs und importiere erneut.",
		textlang.Vietnamese: "Chưa có tỷ giá {currency} cho ngày deal này đóng nên hàng bị bỏ qua. Hãy thêm tỷ giá rồi nhập lại.",
	}},
	ReasonDealLegalGate: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "A stage before this deal’s stage needs a legal review, so the deal cannot be imported as past that stage.",
		textlang.German:     "Eine Phase vor der Phase dieses Deals erfordert eine rechtliche Prüfung, daher kann der Deal nicht über diese Phase hinaus importiert werden.",
		textlang.Vietnamese: "Một giai đoạn trước giai đoạn của deal này cần được rà soát pháp lý nên không thể nhập deal vượt qua giai đoạn đó.",
	}},
	ReasonPipelineNoOpenStage: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "The pipeline has no open stage to start a closed deal on.",
		textlang.German:     "Die Pipeline hat keine offene Phase, in der ein abgeschlossener Deal beginnen kann.",
		textlang.Vietnamese: "Quy trình không có giai đoạn đang mở để bắt đầu một deal đã đóng.",
	}},
	ReasonCannotLinkStakeholders: {params: nil, text: map[textlang.Lang]string{
		textlang.English:    "You cannot add stakeholders to deals, so this row’s contact is not linked.",
		textlang.German:     "Sie dürfen keine Stakeholder zu Deals hinzufügen, daher wird der Kontakt dieser Zeile nicht verknüpft.",
		textlang.Vietnamese: "Bạn không thể thêm bên liên quan vào deal nên liên hệ của hàng này không được liên kết.",
	}},
	ReasonContactNotFound: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No contact with “{ref}” in this upload, in an earlier import, or in Margince.",
		textlang.German:     "Kein Kontakt mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince.",
		textlang.Vietnamese: "Không có liên hệ nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince.",
	}},
	ReasonContactNotFoundDropped: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "No contact with “{ref}” in this upload, in an earlier import, or in Margince. The row was imported without the link.",
		textlang.German:     "Kein Kontakt mit {ref:q} in diesem Upload, in einem früheren Import oder in Margince. Die Zeile wurde ohne Verknüpfung importiert.",
		textlang.Vietnamese: "Không có liên hệ nào với {ref:q} trong tệp này, trong lần nhập trước hoặc trong Margince. Hàng đã được nhập mà không có liên kết.",
	}},
}
