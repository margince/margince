// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

import (
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// labels is what the mapping step calls each core target, in the words the
// record pages use for the same field. A target with no entry here would show
// its internal name, so withLabels refuses one at start-up.
var labels = map[string]langcopy.Phrase{
	"activity.owner_email": {En: "Owner (email)", De: "Verantwortlich (E-Mail)", Vi: "Người chịu trách nhiệm (email)"},
	"address.city":         {En: "City", De: "Stadt", Vi: "Thành phố"},
	"address.country":      {En: "Country", De: "Land", Vi: "Quốc gia"},
	"address.line1":        {En: "Address line 1", De: "Adresszeile 1", Vi: "Địa chỉ dòng 1"},
	"address.line2":        {En: "Address line 2", De: "Adresszeile 2", Vi: "Địa chỉ dòng 2"},
	"address.postal_code":  {En: "Postal code", De: "Postleitzahl", Vi: "Mã bưu chính"},
	"address.region":       {En: "Region", De: "Region", Vi: "Khu vực"},
	"amount":               {En: "Value", De: "Wert", Vi: "Giá trị"},
	"author":               {En: "Author", De: "Autor", Vi: "Tác giả"},
	"body":                 {En: "Notes", De: "Notizen", Vi: "Ghi chú"},
	"close_date":           {En: "Expected close date", De: "Erwartetes Abschlussdatum", Vi: "Ngày dự kiến chốt"},
	"company_name":         {En: "Company name", De: "Firmenname", Vi: "Tên công ty"},
	"company_ref":          {En: "Company ref", De: "Firmen-Ref", Vi: "Ref công ty"},
	"company.display_name": {En: "Company name", De: "Firmenname", Vi: "Tên công ty"},
	"company.linkedin_url": {En: "LinkedIn page", De: "LinkedIn-Seite", Vi: "Trang LinkedIn"},
	"contact_ref":          {En: "Contact ref", De: "Kontakt-Ref", Vi: "Ref liên hệ"},
	"contact.company_name": {En: "Company name", De: "Firmenname", Vi: "Tên công ty"},
	"currency":             {En: "Currency", De: "Währung", Vi: "Tiền tệ"},
	"deal.company_ref":     {En: "Company ref", De: "Firmen-Ref", Vi: "Ref công ty"},
	"deal.description":     {En: "Description", De: "Beschreibung", Vi: "Mô tả"},
	"deal.name":            {En: "Deal name", De: "Name des Deals", Vi: "Tên giao dịch"},
	"deal.ref":             {En: "Deal ref", De: "Deal-Ref", Vi: "Ref giao dịch"},
	"description":          {En: "Description", De: "Beschreibung", Vi: "Mô tả"},
	"direction":            {En: "Direction", De: "Richtung", Vi: "Hướng"},
	"domain":               {En: "Domain", De: "Domain", Vi: "Tên miền"},
	"duration_minutes":     {En: "Duration (minutes)", De: "Dauer (Minuten)", Vi: "Thời lượng (phút)"},
	"email":                {En: "Email address", De: "E-Mail-Adresse", Vi: "Địa chỉ email"},
	"first_name":           {En: "First name", De: "Vorname", Vi: "Tên"},
	"full_name":            {En: "Full name", De: "Vollständiger Name", Vi: "Họ và tên"},
	"id":                   {En: "Margince ID", De: "Margince-ID", Vi: "ID Margince"},
	"industry":             {En: "Industry", De: "Branche", Vi: "Ngành"},
	"last_name":            {En: "Last name", De: "Nachname", Vi: "Họ"},
	"lead.origin":          {En: "Origin", De: "Herkunft", Vi: "Nguồn gốc"},
	"lead.origin_detail":   {En: "Origin detail", De: "Herkunft, Detail", Vi: "Chi tiết nguồn gốc"},
	"lead.status":          {En: "Status", De: "Status", Vi: "Trạng thái"},
	"legal_name":           {En: "Legal name", De: "Rechtlicher Name", Vi: "Tên pháp lý"},
	"linkedin_url":         {En: "LinkedIn page", De: "LinkedIn-Seite", Vi: "Trang LinkedIn"},
	"links":                {En: "Linked records", De: "Verknüpfte Datensätze", Vi: "Hồ sơ liên kết"},
	"lost_reason":          {En: "Lost reason", De: "Verlustgrund", Vi: "Lý do thua"},
	"meeting_status":       {En: "Meeting status", De: "Besprechungsstatus", Vi: "Trạng thái cuộc họp"},
	"occurred_at":          {En: "Date and time", De: "Datum und Uhrzeit", Vi: "Ngày giờ"},
	"owner_email":          {En: "Owner (email)", De: "Verantwortlich (E-Mail)", Vi: "Người chịu trách nhiệm (email)"},
	"phone":                {En: "Phone", De: "Telefon", Vi: "Điện thoại"},
	"pipeline":             {En: "Pipeline", De: "Pipeline", Vi: "Pipeline"},
	"ref":                  {En: "Your own key (ref)", De: "Dein eigener Schlüssel (Ref)", Vi: "Khóa riêng của bạn (ref)"},
	"size_band":            {En: "Employees", De: "Mitarbeitende", Vi: "Số nhân viên"},
	"stage":                {En: "Stage", De: "Phase", Vi: "Giai đoạn"},
	"subject":              {En: "Subject", De: "Betreff", Vi: "Tiêu đề"},
	"tags":                 {En: "Tags", De: "Tags", Vi: "Thẻ"},
	"title":                {En: "Job title", De: "Position", Vi: "Chức danh"},
	"type":                 {En: "Activity type", De: "Art der Aktivität", Vi: "Loại hoạt động"},
}

// withLabels gives every column its label. It panics at start-up on a column
// with none, because the mapping step would then show that column's internal
// name to somebody choosing where their data goes.
func withLabels(columns map[string]Column) map[string]Column {
	for key, col := range columns {
		label, ok := labels[key]
		if !ok {
			panic("importguide: no label for the import target " + key)
		}
		col.Label = label
		columns[key] = col
	}
	return columns
}
