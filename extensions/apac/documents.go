// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apac

import (
	_ "embed"

	"github.com/margince/margince/backend/pkg/extension/compliancedocs"
)

//go:embed documents/vn/vi/thong-bao-cho-nguoi-lao-dong.md
var employeeNotice []byte

//go:embed documents/vn/en/employee-notice.md
var employeeNoticeReadingCopy []byte

//go:embed documents/vn/vi/xac-nhan-da-nhan-thong-bao.md
var noticeReceipt []byte

func vietnameseDocuments() []compliancedocs.Document {
	return []compliancedocs.Document{
		{
			Jurisdiction: "vn", ID: "thong-bao-cho-nguoi-lao-dong",
			Title:    "Thông báo cho người lao động về việc lưu email công việc",
			Language: "vi", Audience: compliancedocs.Employee, Kind: compliancedocs.Template,
			Body: employeeNotice, MediaType: "text/markdown",
		},
		{
			Jurisdiction: "vn", ID: "employee-notice",
			Title:    "Employee notice about capturing work email",
			Language: "en", Audience: compliancedocs.Employee, Kind: compliancedocs.ReadingCopy,
			Body: employeeNoticeReadingCopy, MediaType: "text/markdown",
		},
		{
			Jurisdiction: "vn", ID: "xac-nhan-da-nhan-thong-bao",
			Title:    "Xác nhận đã nhận thông báo về việc lưu email công việc",
			Language: "vi", Audience: compliancedocs.Employee, Kind: compliancedocs.Template,
			Body: noticeReceipt, MediaType: "text/markdown",
		},
	}
}
