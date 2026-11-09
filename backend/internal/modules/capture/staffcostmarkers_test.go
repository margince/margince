// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

func TestADocumentNamingStaffWithAmountsIsPersonnel(t *testing.T) {
	for name, text := range map[string]string{
		"an invoice billing engineers by name, names wrapped as a PDF wraps them": `INVOICE 1042
ACTIVITY QTY RATE AMOUNT
Development Services Lead PHP Arjun Patel 1 8,464.00 8,464.00
Development Services Standard Frontend Lan Vo
Thi
1 3,136.00 3,136.00
Development Services Senior++ PHP Minh
Tran Duc
1 5,726.00 5,726.00
BALANCE DUE USD 17,326.00`,
		"an invoice billing one named engineer": `INVOICE 1043 SAP
ACTIVITY QTY RATE AMOUNT
Development Services Senior PHP Quang Le
Van
Northwind SAP ERP Support, project R.I.D.E.
1 6,212.00 6,212.00
BALANCE DUE USD 6,212.00`,
		"a cost-plus sheet with no word for pay": `Sheet: Cost Plus - Calculation
Level | Stack | Jan | Feb | Mar
Bùi Văn An | Senior Frontend | 4990 | 4990 | 4651
Nguyễn Thị Hoa | Senior++ Frontend | 5990 | 5990 | 5806
Trần Minh Đức | QC | 4590 | 4590 | 6053`,
	} {
		if !ListsStaffCosts(text) {
			t.Errorf("%s: not held", name)
		}
	}
}

func TestOrdinaryTradeDocumentsAreNotPersonnel(t *testing.T) {
	for name, text := range map[string]string{
		"a rate card that names roles and nobody": `Senior Developer | 650.00 per day
Junior Developer | 420.00 per day
Project Manager | 780.00 per day
QA Engineer | 500.00 per day`,
		"a supplier's invoice": `INVOICE 88
Microsoft 365 Business Premium 12 users 264.00 3,168.00
Hosting March 1 450.00 450.00`,
		"a payroll software offer":                        "Payroll integration planned for 2026. Payroll Software 500.00 per month",
		"a marketing invoice whose line starts with Lead": `Lead generation Campaign Google Ads 1 500.00 500.00`,
		"an email signature with a phone number": `Best regards
Mi Le
Junior Finance & Accounting
Tel +84 2838 123456`,
		"one consultant's line among ordinary ones": `Senior Consultant Peter Miller 10
Travel 1 120`,
	} {
		if ListsStaffCosts(text) {
			t.Errorf("%s: held, but it states nobody's pay", name)
		}
	}
}
