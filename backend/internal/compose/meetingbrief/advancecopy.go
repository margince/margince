// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var advanceWords = struct {
	GroundedMinimum      langcopy.Phrase
	GroundedBest         langcopy.Phrase
	GroundedFallback     langcopy.Phrase
	Them                 langcopy.Phrase
	RelationshipMinimum  langcopy.Phrase
	RelationshipBest     langcopy.Phrase
	RelationshipFallback langcopy.Phrase
	DiscoveryMinimum     langcopy.Phrase
	DiscoveryBest        langcopy.Phrase
	DiscoveryFallback    langcopy.Phrase
	DemoMinimum          langcopy.Phrase
	DemoBest             langcopy.Phrase
	DemoFallback         langcopy.Phrase
	CommercialMinimum    langcopy.Phrase
	CommercialBest       langcopy.Phrase
	CommercialFallback   langcopy.Phrase
	DecisionMinimum      langcopy.Phrase
	DecisionBest         langcopy.Phrase
	DecisionFallback     langcopy.Phrase
	DeliveryMinimum      langcopy.Phrase
	DeliveryBest         langcopy.Phrase
	DeliveryFallback     langcopy.Phrase
	RenewalMinimum       langcopy.Phrase
	RenewalBest          langcopy.Phrase
	RenewalFallback      langcopy.Phrase
	Social               langcopy.Phrase
	SocialPlay           langcopy.Phrase
	Work                 langcopy.Phrase
	WorkPlay             langcopy.Phrase
	Complaint            langcopy.Phrase
	ComplaintPlay        langcopy.Phrase
	Price                langcopy.Phrase
	PricePlay            langcopy.Phrase
	Stakeholder          langcopy.Phrase
	StakeholderPlay      langcopy.Phrase
	Slip                 langcopy.Phrase
	SlipPlay             langcopy.Phrase
	Escalation           langcopy.Phrase
	EscalationPlay       langcopy.Phrase
	Scope                langcopy.Phrase
	ScopePlay            langcopy.Phrase
	EarlyDemo            langcopy.Phrase
	EarlyDemoPlay        langcopy.Phrase
	NoProblem            langcopy.Phrase
	NoProblemPlay        langcopy.Phrase
}{
	GroundedMinimum:      langcopy.Phrase{En: "Leave with one named next step, owned and dated.", De: "Vereinbare einen konkreten nächsten Schritt mit verantwortlicher Person und Termin.", Vi: "Kết thúc với một bước tiếp theo cụ thể, có người phụ trách và ngày thực hiện."},
	GroundedBest:         langcopy.Phrase{En: "Leave with the next meeting booked and its purpose agreed.", De: "Vereinbare das nächste Meeting und seinen Zweck.", Vi: "Kết thúc với cuộc họp tiếp theo đã được đặt lịch và mục đích đã thống nhất."},
	GroundedFallback:     langcopy.Phrase{En: "Leave with what you will send and who decides on it.", De: "Kläre, was du schickst und wer darüber entscheidet.", Vi: "Kết thúc với sự thống nhất về những gì bạn sẽ gửi và ai quyết định về chúng."},
	Them:                 langcopy.Phrase{En: "them", De: "ihnen", Vi: "họ"},
	RelationshipMinimum:  langcopy.Phrase{En: "A named next step with an owner and a date, even if the answer is 'not this quarter'.", De: "Ein konkreter nächster Schritt mit verantwortlicher Person und Termin, auch wenn die Antwort „nicht in diesem Quartal“ lautet.", Vi: "Một bước tiếp theo cụ thể với người phụ trách và ngày thực hiện, ngay cả khi câu trả lời là “không phải quý này”."},
	RelationshipBest:     langcopy.Phrase{En: "A working session booked with %s and whoever else has to be in it.", De: "Ein vereinbarter Arbeitstermin mit %s und allen weiteren nötigen Beteiligten.", Vi: "Một buổi làm việc được đặt lịch với %s và những người khác cần tham gia."},
	RelationshipFallback: langcopy.Phrase{En: "Agreement on what you will send, and who decides what happens with it.", De: "Eine Vereinbarung darüber, was du schickst und wer über das weitere Vorgehen entscheidet.", Vi: "Thống nhất bạn sẽ gửi gì và ai quyết định bước xử lý tiếp theo."},
	DiscoveryMinimum:     langcopy.Phrase{En: "One quantified problem, and who owns it today.", De: "Ein beziffertes Problem und die Person, die heute dafür verantwortlich ist.", Vi: "Một vấn đề được định lượng và người đang chịu trách nhiệm về nó."},
	DiscoveryBest:        langcopy.Phrase{En: "The two outcomes that matter most, and a booked session to scope them.", De: "Die zwei wichtigsten Ergebnisse und ein vereinbarter Termin, um ihren Umfang zu klären.", Vi: "Hai kết quả quan trọng nhất và một buổi đã đặt lịch để xác định phạm vi."},
	DiscoveryFallback:    langcopy.Phrase{En: "A written summary they agree with, and the name of the colleague who has to approve it.", De: "Eine schriftliche Zusammenfassung, der sie zustimmen, und der Name der Person, die sie freigeben muss.", Vi: "Một bản tóm tắt bằng văn bản mà họ đồng ý và tên đồng nghiệp cần phê duyệt."},
	DemoMinimum:          langcopy.Phrase{En: "Their reaction to the two things they said they needed.", De: "Ihre Reaktion auf die zwei Dinge, die sie als nötig benannt haben.", Vi: "Phản hồi của họ về hai điều họ nói là cần."},
	DemoBest:             langcopy.Phrase{En: "Agreement that it fits, and a date to talk commercials.", De: "Einigkeit, dass es passt, und ein Termin für die kommerziellen Fragen.", Vi: "Thống nhất rằng giải pháp phù hợp và một ngày để bàn điều khoản thương mại."},
	DemoFallback:         langcopy.Phrase{En: "The one objection that stopped it, in their own words.", De: "Der entscheidende Einwand, in ihren eigenen Worten.", Vi: "Ý kiến phản đối đã khiến việc này dừng lại, bằng chính lời của họ."},
	CommercialMinimum:    langcopy.Phrase{En: "The remaining gap between our number and theirs, stated.", De: "Die noch bestehende Differenz zwischen unserer Zahl und ihrer, klar benannt.", Vi: "Nêu rõ khoảng cách còn lại giữa con số của chúng ta và của họ."},
	CommercialBest:       langcopy.Phrase{En: "Agreed terms and a signature date.", De: "Vereinbarte Konditionen und ein Unterschriftstermin.", Vi: "Các điều khoản đã thống nhất và ngày ký."},
	CommercialFallback:   langcopy.Phrase{En: "What has to be true for them to sign, and by when.", De: "Was bis wann erfüllt sein muss, damit sie unterschreiben.", Vi: "Điều gì cần được đáp ứng để họ ký và hạn chót là khi nào."},
	DecisionMinimum:      langcopy.Phrase{En: "The date the decision gets made, and who makes it.", De: "Der Entscheidungstermin und die entscheidende Person.", Vi: "Ngày ra quyết định và người quyết định."},
	DecisionBest:         langcopy.Phrase{En: "The decision itself.", De: "Die Entscheidung selbst.", Vi: "Chính quyết định đó."},
	DecisionFallback:     langcopy.Phrase{En: "The one unresolved objection, and who has to answer it.", De: "Der noch offene Einwand und die Person, die ihn beantworten muss.", Vi: "Ý kiến phản đối chưa được giải quyết và người cần trả lời."},
	DeliveryMinimum:      langcopy.Phrase{En: "Agreement on what is done and what is next.", De: "Einigkeit darüber, was erledigt ist und was als Nächstes ansteht.", Vi: "Thống nhất việc gì đã xong và việc gì tiếp theo."},
	DeliveryBest:         langcopy.Phrase{En: "The next milestone dated, with an owner on each side.", De: "Der nächste Meilenstein mit Termin und je einer verantwortlichen Person auf beiden Seiten.", Vi: "Mốc tiếp theo có ngày cụ thể và một người phụ trách ở mỗi bên."},
	DeliveryFallback:     langcopy.Phrase{En: "A written list of what is blocked and who unblocks it.", De: "Eine schriftliche Liste der Hindernisse und der Personen, die sie beseitigen.", Vi: "Một danh sách bằng văn bản về những việc bị chặn và ai sẽ tháo gỡ."},
	RenewalMinimum:       langcopy.Phrase{En: "One thing that changes before the renewal date.", De: "Eine Änderung vor dem Verlängerungstermin.", Vi: "Một thay đổi trước ngày gia hạn."},
	RenewalBest:          langcopy.Phrase{En: "Agreement to renew, or the date that decision gets made.", De: "Die Zusage zur Verlängerung oder der Termin für diese Entscheidung.", Vi: "Thống nhất gia hạn hoặc ngày đưa ra quyết định đó."},
	RenewalFallback:      langcopy.Phrase{En: "What went wrong, in their words, and who else needs to hear it.", De: "Was schiefgelaufen ist, in ihren Worten, und wer es noch hören muss.", Vi: "Điều gì đã sai, bằng lời của họ, và ai khác cần nghe điều đó."},
	Social:               langcopy.Phrase{En: "It stays social", De: "Es bleibt ein persönliches Gespräch", Vi: "Cuộc gặp vẫn mang tính giao lưu"},
	SocialPlay:           langcopy.Phrase{En: "Let it. Ask one 'what has changed' question and earn a separate working session.", De: "Lass es dabei. Frage einmal, was sich verändert hat, und gewinne einen eigenen Arbeitstermin.", Vi: "Hãy để như vậy. Hỏi một câu về điều gì đã thay đổi và tạo cơ sở cho một buổi làm việc riêng."},
	Work:                 langcopy.Phrase{En: "They raise the work", De: "Sie sprechen die Arbeit an", Vi: "Họ đề cập đến công việc"},
	WorkPlay:             langcopy.Phrase{En: "Follow it: prioritise two outcomes, then propose a scoping session.", De: "Greife es auf: Priorisiere zwei Ergebnisse und schlage dann einen Termin zur Umfangsklärung vor.", Vi: "Đi theo chủ đề đó: ưu tiên hai kết quả rồi đề xuất một buổi xác định phạm vi."},
	Complaint:            langcopy.Phrase{En: "They raise a complaint", De: "Sie sprechen eine Beschwerde an", Vi: "Họ nêu một khiếu nại"},
	ComplaintPlay:        langcopy.Phrase{En: "Hear it out fully before answering. Do not defend the internal history.", De: "Höre alles an, bevor du antwortest. Verteidige nicht die internen Abläufe der Vergangenheit.", Vi: "Nghe hết trước khi trả lời. Đừng biện hộ cho những gì đã xảy ra nội bộ."},
	Price:                langcopy.Phrase{En: "It becomes a price negotiation", De: "Es wird eine Preisverhandlung", Vi: "Cuộc họp trở thành đàm phán giá"},
	PricePlay:            langcopy.Phrase{En: "Anchor on the outcome agreed, not the list. Trade scope for price, never price alone.", De: "Orientiere dich am vereinbarten Ergebnis, nicht an der Preisliste. Tausche Umfang gegen Preis, gib nie nur beim Preis nach.", Vi: "Dựa vào kết quả đã thống nhất, không phải bảng giá. Điều chỉnh phạm vi để đổi lấy giá, không chỉ giảm giá đơn thuần."},
	Stakeholder:          langcopy.Phrase{En: "A new stakeholder appears", De: "Eine neue beteiligte Person kommt hinzu", Vi: "Một bên liên quan mới xuất hiện"},
	StakeholderPlay:      langcopy.Phrase{En: "Stop selling and qualify them: what do they need to believe, and what do they decide?", De: "Unterbrich den Verkauf und kläre ihre Rolle: Wovon müssen sie überzeugt sein und was entscheiden sie?", Vi: "Dừng chào bán và làm rõ vai trò của họ: họ cần tin vào điều gì và họ quyết định điều gì?"},
	Slip:                 langcopy.Phrase{En: "The decision slips", De: "Die Entscheidung verschiebt sich", Vi: "Quyết định bị lùi lại"},
	SlipPlay:             langcopy.Phrase{En: "Get the new date and the reason in the room, before it becomes an email.", De: "Kläre den neuen Termin und den Grund noch im Meeting, bevor daraus eine E-Mail wird.", Vi: "Lấy ngày mới và lý do ngay trong cuộc họp, trước khi việc này chuyển thành trao đổi qua email."},
	Escalation:           langcopy.Phrase{En: "It becomes an escalation", De: "Es kommt zur Eskalation", Vi: "Vấn đề được đưa lên cấp cao hơn"},
	EscalationPlay:       langcopy.Phrase{En: "Take it seriously in the room. Name what you will fix and by when; commit to nothing else.", De: "Nimm es im Meeting ernst. Benenne, was du bis wann behebst, und sage nichts darüber hinaus zu.", Vi: "Nghiêm túc xử lý ngay trong cuộc họp. Nêu rõ sẽ sửa gì và khi nào; không cam kết thêm điều gì khác."},
	Scope:                langcopy.Phrase{En: "They ask for more scope", De: "Sie wünschen zusätzlichen Umfang", Vi: "Họ yêu cầu mở rộng phạm vi"},
	ScopePlay:            langcopy.Phrase{En: "Welcome it and price it. Agreeing to it here is how a delivery goes red.", De: "Nimm den Wunsch auf und bepreise ihn. Eine ungeprüfte Zusage im Meeting gefährdet die Umsetzung.", Vi: "Đón nhận yêu cầu và định giá. Đồng ý ngay tại đây có thể khiến việc triển khai gặp rủi ro."},
	EarlyDemo:            langcopy.Phrase{En: "They want the demo early", De: "Sie wollen die Demo vorziehen", Vi: "Họ muốn xem demo sớm"},
	EarlyDemoPlay:        langcopy.Phrase{En: "Show the two things they named, then return to what problem they were for.", De: "Zeige die zwei genannten Dinge und kehre dann zu dem Problem zurück, das sie lösen sollen.", Vi: "Trình bày hai điều họ đã nêu rồi quay lại vấn đề mà chúng cần giải quyết."},
	NoProblem:            langcopy.Phrase{En: "They will not name a problem", De: "Sie nennen kein Problem", Vi: "Họ không nêu rõ vấn đề"},
	NoProblemPlay:        langcopy.Phrase{En: "Ask what happens today, step by step, and listen for where it costs them.", De: "Frage Schritt für Schritt nach dem heutigen Ablauf und höre darauf, wo er sie etwas kostet.", Vi: "Hỏi từng bước về cách mọi việc diễn ra hiện nay và lắng nghe xem chỗ nào gây tốn kém cho họ."},
}
