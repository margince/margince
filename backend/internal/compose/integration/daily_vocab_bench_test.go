// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"math/rand/v2"
	"strings"
)

// German, English and Vietnamese, so "co", "con", "contr" match across words as in a real
// inbox. Every name is assembled from these parts; none names anyone real.
var (
	dailyGermanFirst = []string{
		"Anna", "Lukas", "Jonas", "Lena", "Felix", "Marie", "Paul", "Sophie", "Jan", "Laura",
		"Tobias", "Katrin", "Stefan", "Julia", "Matthias", "Sabine", "Jürgen", "Ute", "Björn", "Hannah",
		"Florian", "Carola", "Dominik", "Grete", "Henrik", "Ines", "Konrad", "Mareike", "Nils", "Petra",
	}
	dailyGermanLast = []string{
		"Müller", "Schmidt", "Schneider", "Fischer", "Weber", "Meyer", "Wagner", "Becker",
		"Schulz", "Hoffmann", "Koch", "Richter", "Wolf", "Schröder", "Neumann", "Schwarz", "Zimmermann",
		"Braun", "Krüger", "Hartmann", "Lange", "Werner", "Krause", "Lehmann", "Köhler",
	}
	dailyEnglishFirst = []string{
		"James", "Emily", "Oliver", "Grace", "Thomas", "Chloe", "Daniel", "Sarah",
		"Michael", "Olivia", "Harry", "Amelia", "George", "Isla", "Samuel", "Megan", "Connor", "Ruth", "Ethan", "Holly",
	}
	dailyEnglishLast = []string{
		"Smith", "Johnson", "Brown", "Taylor", "Wilson", "Evans", "Walker", "Wright",
		"Clarke", "Hughes", "Cooper", "Turner", "Parker", "Collins", "Morgan",
	}
	dailyVietFirst = []string{
		"Minh", "Linh", "Hương", "Tuấn", "Anh", "Thảo", "Đức", "Lan", "Hiếu", "Trang",
		"Quang", "Ngọc", "Phương", "Hải", "Thủy", "Khánh", "Long", "Mai", "Bảo", "Vy",
	}
	dailyVietLast = []string{
		"Nguyễn", "Trần", "Lê", "Phạm", "Hoàng", "Phan", "Vũ", "Đặng", "Bùi", "Đỗ",
		"Hồ", "Ngô", "Dương", "Lý", "Võ", "Trương", "Đinh", "Lâm",
	}

	dailyCompanyStems = []string{
		"Müller", "Nordlicht", "Rhein", "Alpen", "Hanse", "Elbe", "Bavaria", "Schwarzwald",
		"Saigon", "Mekong", "Hanoi", "Sông Hồng", "Đà Nẵng", "Thames", "Pennine", "Harbour", "Northgate",
		"Kontor", "Contura", "Conrad", "Continental", "Lindner", "Brückner", "Westfalen", "Atlas", "Meridian",
	}
	dailyCompanySectors = []string{
		"Logistik", "Maschinenbau", "Software", "Trading", "Consulting", "Foods",
		"Textil", "Energie", "Pharma", "Bau", "Systems", "Solutions", "Digital", "Handel", "Controls", "Contracting",
	}
	dailyCompanySuffixes = []string{"GmbH", "AG", "GmbH & Co. KG", "KG", "Ltd", "Co", "JSC", "Co., Ltd.", "plc", "SE"}
	dailyIndustries      = []string{
		"Logistics", "Manufacturing", "Software", "Wholesale", "Professional services",
		"Food and beverage", "Textiles", "Energy", "Life sciences", "Construction",
	}
	dailyTitles = []string{
		"Geschäftsführer", "Einkaufsleiterin", "Head of Procurement", "CFO", "Operations Manager",
		"Projektleiter", "Sales Director", "Giám đốc", "Trưởng phòng mua hàng", "IT-Leiter", "Controller", "Buyer",
	}

	dailyDealWords = []string{
		"Rahmenvertrag", "Wartungsvertrag", "Lizenz", "Angebot", "Rollout", "Pilot",
		"Renewal", "Expansion", "Implementation", "Support contract", "Hợp đồng bảo trì", "Contract extension",
		"Consulting retainer", "Controlling Suite",
	}
	dailyProjectWords = []string{
		"Migration", "Einführung", "Rollout", "Phase 2", "Integration", "Onboarding",
		"Triển khai", "Upgrade",
	}
	dailyLeadCompanies = []string{
		"Kontor Nord", "Saigon Freight", "Pennine Foods", "Alpen Bau", "Mekong Textile",
		"Rhein Digital", "Harbour Systems", "Elbe Pharma", "Hanoi Controls", "Atlas Contracting",
	}

	dailySubjects = []string{
		"Angebot für den Rahmenvertrag", "Vertrag zur Unterschrift", "Termin nächste Woche",
		"Proposal for the contract renewal", "Invoice and payment terms", "Meeting notes and next steps",
		"Báo giá hợp đồng mới", "Lịch họp tuần sau", "Rückfrage zur Rechnung", "Contract questions before signing",
		"Confirmation of the delivery schedule", "Kündigung und Verlängerung", "Controlling report Q3",
	}
	dailyTaskSubjects = []string{
		"Angebot nachfassen", "Send the revised proposal", "Vertrag prüfen",
		"Call back about the invoice", "Gửi báo giá", "Prepare the contract draft", "Termin bestätigen",
	}
)

var dailySentences = map[string][]string{
	"de": {
		"Anbei das aktualisierte Angebot für den Rahmenvertrag, wie im Termin besprochen.",
		"Können wir den Termin für das Meeting am Dienstag um 14 Uhr bestätigen?",
		"Die Rechnung für das letzte Quartal ist noch offen, bitte prüfen Sie den Betrag.",
		"Unser Controlling braucht vor der Unterschrift eine Kopie des Vertrags.",
		"Wir würden den Wartungsvertrag gern um zwei Jahre verlängern.",
		"Der Lieferplan verschiebt sich um eine Woche, die Konditionen bleiben gleich.",
		"Bitte senden Sie uns die Kontaktdaten Ihrer Ansprechpartnerin im Einkauf.",
		"Das Angebot gilt bis Monatsende, danach müssen wir die Preise neu kalkulieren.",
		"Im Anhang finden Sie das Protokoll unseres Gesprächs und die nächsten Schritte.",
		"Die Geschäftsführung hat dem Pilotprojekt grundsätzlich zugestimmt.",
		"Wir haben noch Rückfragen zu Paragraph sieben des Vertragsentwurfs.",
		"Könnten Sie die Rechnung bitte auf unsere neue Adresse ausstellen?",
	},
	"en": {
		"Please find attached the revised proposal for the contract renewal.",
		"Could we confirm the meeting on Tuesday at two to go through the invoice?",
		"Our controller needs a signed copy of the contract before the order is released.",
		"We would like to extend the support contract by another twelve months.",
		"The delivery schedule moves by one week; pricing and terms stay as agreed.",
		"Can you connect me with the right contact in your procurement team?",
		"The proposal is valid until the end of the month, after which prices are reviewed.",
		"Attached are the notes from our call and the next steps we agreed on.",
		"The board approved the pilot in principle and asked for a rollout plan.",
		"We still have a few questions about clause seven of the draft contract.",
		"Could you reissue the invoice to our new billing address, please?",
		"Thanks for the constructive conversation yesterday, let us continue next week.",
	},
	"vi": {
		"Gửi anh bản báo giá mới cho hợp đồng bảo trì như đã trao đổi.",
		"Chúng ta có thể xác nhận lịch họp vào thứ Ba lúc hai giờ chiều không?",
		"Phòng kế toán cần bản hợp đồng đã ký trước khi thanh toán hóa đơn.",
		"Chúng tôi muốn gia hạn hợp đồng hỗ trợ thêm mười hai tháng.",
		"Lịch giao hàng lùi một tuần, giá và điều khoản giữ nguyên.",
		"Anh có thể giới thiệu người phụ trách mua hàng bên công ty không?",
		"Báo giá có hiệu lực đến cuối tháng, sau đó chúng tôi sẽ điều chỉnh giá.",
		"Đính kèm là biên bản cuộc họp và các bước tiếp theo.",
		"Ban giám đốc đã đồng ý triển khai thí điểm và cần kế hoạch chi tiết.",
		"Chúng tôi còn vài câu hỏi về điều khoản số bảy trong dự thảo hợp đồng.",
	},
}

// dailyContactNames pairs names within one culture, so a contact reads as one culture's name
// and writes in its language; a culture weighs as many pairs as its parts make.
func dailyContactNames() (firsts, lasts, languages []string) {
	for _, culture := range []struct {
		language    string
		first, last []string
	}{
		{"de", dailyGermanFirst, dailyGermanLast},
		{"en", dailyEnglishFirst, dailyEnglishLast},
		{"vi", dailyVietFirst, dailyVietLast},
	} {
		for _, first := range culture.first {
			for _, last := range culture.last {
				firsts = append(firsts, first)
				lasts = append(lasts, last)
				languages = append(languages, culture.language)
			}
		}
	}
	return firsts, lasts, languages
}

// dailyParagraphs builds n one-kilobyte paragraphs from a fixed seed, so two runs index
// the same text and a body's size is the number of paragraphs it draws.
func dailyParagraphs(language string, n int) []string {
	sentences := dailySentences[language]
	rng := rand.New(rand.NewPCG(42, uint64(len(language))))
	paragraphs := make([]string, n)
	for i := range paragraphs {
		var b strings.Builder
		for b.Len() < 900 {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(sentences[rng.IntN(len(sentences))])
		}
		paragraphs[i] = b.String()
	}
	return paragraphs
}
