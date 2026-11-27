// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The refusals of an id cell on a contact, lead or deal row. A company row keeps
// the company codes, which clients already know. These sit apart from the
// catalog to keep each file under the length cap.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Reason codes for an id cell on a record that is not a company.
const (
	ReasonIDNotARecordID          ReasonCode = "id_not_a_record_id"
	ReasonIDRecordNotVisible      ReasonCode = "id_record_not_visible"
	ReasonRefTakenByAnotherRecord ReasonCode = "ref_taken_by_another_record"
	ReasonNotYourRecord           ReasonCode = "not_your_record"
)

// withRecordIDReasons adds the record id reasons to the catalog and answers it.
func withRecordIDReasons(catalog map[ReasonCode]reasonEntry) map[ReasonCode]reasonEntry {
	maps.Copy(catalog, recordIDReasons)
	return catalog
}

var recordIDReasons = map[ReasonCode]reasonEntry{
	ReasonNotYourRecord: {text: map[textlang.Lang]string{
		textlang.English:    "This row matches a record another member owns. Your import changes only records you own, so the row is not applied.",
		textlang.German:     "Diese Zeile passt zu einem Datensatz, der einem anderen Mitglied gehört. Ihr Import ändert nur Ihre eigenen Datensätze, daher wird die Zeile nicht übernommen.",
		textlang.Vietnamese: "Hàng này khớp với một bản ghi do thành viên khác phụ trách. Lần nhập của bạn chỉ thay đổi bản ghi bạn phụ trách nên hàng này không được áp dụng.",
	}},
	ReasonIDNotARecordID: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "“{value}” is not a record id. Export the list to get the ids, or leave the column empty to import this row as a new record.",
		textlang.German:     "{value:q} ist keine Datensatz-ID. Exportieren Sie die Liste, um die IDs zu erhalten, oder lassen Sie die Spalte leer, um die Zeile als neuen Datensatz zu importieren.",
		textlang.Vietnamese: "{value:q} không phải mã bản ghi. Hãy xuất danh sách để lấy mã, hoặc để trống cột để nhập hàng này làm bản ghi mới.",
	}},
	ReasonIDRecordNotVisible: {params: []string{paramValue}, text: map[textlang.Lang]string{
		textlang.English:    "No record you can see has the id “{value}”. It may have been archived or merged away, or come from a different system.",
		textlang.German:     "Kein Datensatz, den Sie sehen können, hat die ID {value:q}. Er kann archiviert oder zusammengeführt worden sein oder aus einem anderen System stammen.",
		textlang.Vietnamese: "Không có bản ghi nào bạn nhìn thấy có mã {value:q}. Có thể nó đã được lưu trữ hoặc gộp, hoặc đến từ một hệ thống khác.",
	}},
	ReasonRefTakenByAnotherRecord: {params: []string{paramRef}, text: map[textlang.Lang]string{
		textlang.English:    "The reference “{ref}” already stands for another record from an earlier import, so this row is not applied. Use a new reference for this row or fix the id column.",
		textlang.German:     "Die Referenz {ref:q} steht bereits für einen anderen Datensatz aus einem früheren Import, daher wird diese Zeile nicht übernommen. Verwenden Sie eine neue Referenz für diese Zeile oder korrigieren Sie die ID-Spalte.",
		textlang.Vietnamese: "Tham chiếu {ref:q} đã đại diện cho một bản ghi khác từ lần nhập trước nên hàng này không được áp dụng. Hãy dùng tham chiếu mới cho hàng này hoặc sửa cột mã.",
	}},
}
