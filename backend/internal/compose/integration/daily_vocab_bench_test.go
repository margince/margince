// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// German, English and Vietnamese names, so a prefix such as "co" or "con" matches across
// records as in a real CRM. Every name is assembled from these parts; none names anyone real.
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
	// dailyTaskVerbs open a task's subject; what the task is about follows them.
	dailyTaskVerbs = []string{"Nachfassen", "Follow up", "Rückruf", "Prüfen", "Gửi lại", "Send", "Vorbereiten", "Gọi lại"}
)

// dailyTerm is a word the searches ask for and the share of activities whose
// subject or body names it. Generated text never spells one, so the share holds.
type dailyTerm struct {
	Word, Language string
	Share          float64
}

// The shares are the bench's own choice: a business word names a few percent of
// all mail, so a search for it ranks thousands of rows, not most of the table.
var dailyTerms = []dailyTerm{
	{"contract", "en", 0.05},
	{"Vertrag", "de", 0.03},
	{"Angebot", "de", 0.03},
	{"meeting", "en", 0.10},
	{"invoice", "en", 0.04},
	{"proposal", "en", 0.03},
	{"Termin", "de", 0.03},
}

// dailyFunctionWords are each language's commonest words, commonest first. Real
// ones, so "co" (could, có, company) and "con" (còn, confirm) match most mail.
var dailyFunctionWords = map[string][]string{
	"en": {
		"the", "to", "and", "of", "a", "in", "for", "you", "we", "is", "on", "that", "this", "with", "be", "it",
		"are", "as", "at", "your", "will", "have", "our", "can", "from", "please", "by", "or", "not", "if", "us",
		"could", "would", "all", "an", "next", "week", "thanks", "regards", "let", "me", "know", "also", "any",
		"about", "team", "call", "time", "need", "send", "see", "so", "they", "which", "should", "get", "new",
		"come", "contact", "confirm", "company", "cost", "copy", "course", "current", "today", "tomorrow",
		"best", "more", "sure", "just", "still", "after", "before", "agreed", "attached", "update", "order",
		"price", "delivery", "project", "questions", "office", "back", "there", "were", "been", "here",
	},
	"de": {
		"die", "der", "und", "in", "zu", "den", "das", "nicht", "von", "sie", "ist", "des", "sich", "mit", "dem",
		"dass", "er", "es", "ein", "ich", "auf", "so", "eine", "auch", "als", "an", "nach", "wie", "im", "für",
		"Sie", "Ihnen", "wir", "uns", "bitte", "Grüße", "freundlichen", "gerne", "Woche", "heute", "morgen",
		"noch", "bis", "oder", "aber", "vor", "zur", "mehr", "durch", "unser", "Ihre", "können", "würden",
		"haben", "werden", "wird", "sind", "Dank", "vielen", "Kosten", "Lieferung", "Rechnung", "Unterlagen",
		"Projekt", "Preis", "Rückfrage", "kurz", "nächste", "Anhang", "beigefügt", "Abstimmung", "Kunde",
	},
	"vi": {
		"và", "của", "có", "là", "không", "được", "cho", "các", "những", "với", "này", "một", "trong", "đã", "sẽ",
		"để", "khi", "cũng", "anh", "chị", "em", "chúng", "tôi", "ta", "bạn", "về", "như", "đến", "từ", "theo",
		"còn", "nhưng", "thì", "rất", "nhiều", "đó", "lại", "ra", "vào", "nên", "hay", "sau", "trước", "tuần",
		"ngày", "tháng", "gửi", "nhé", "cảm", "ơn", "công", "ty", "giá", "hàng", "giao", "kế", "hoạch", "dự",
		"án", "xác", "nhận", "liên", "hệ", "thêm", "mới", "bên", "phía", "đơn", "chi", "phí", "tài", "liệu",
	},
}

// dailySyllables build the content words. "co", "con" and "com" open a few of
// them as in real text; none opens with "tr", so only a term spells "contr".
var dailySyllables = []string{
	"ba", "be", "bi", "bo", "bu", "da", "de", "di", "do", "du", "fa", "fe", "fi", "fo", "ga", "ge", "gi", "go",
	"ha", "he", "hi", "ho", "ka", "ke", "ki", "ku", "la", "le", "li", "lo", "lu", "ma", "me", "mi", "mo", "mu",
	"na", "ne", "ni", "no", "nu", "pa", "pe", "pi", "po", "ra", "re", "ri", "ro", "ru", "sa", "se", "si", "so",
	"su", "ta", "te", "ti", "to", "tu", "va", "ve", "vi", "wa", "we", "za", "zu", "ber", "dan", "fen", "gor",
	"han", "kel", "lin", "mar", "nor", "pel", "ran", "sen", "ter", "ung", "ver", "win", "bel", "dor", "mel",
	"ron", "sal", "tin", "co", "con", "com", "cor",
}

// The vocabulary's shape: how many content words exist, how steeply their use
// falls with rank (Zipf's s > 1), and how much of running text is function words.
const (
	dailyContentWords   = 12_000
	dailyContentSkew    = 1.07
	dailyFunctionSkew   = 1.15
	dailyFunctionShare  = 0.5
	dailyParagraphBytes = 900
)

// dailyContentVocabulary lists the generated content words in the order made, with
// any word a term opens dropped so the term table alone decides where a term is.
func dailyContentVocabulary() []string {
	rng := rand.New(rand.NewPCG(7, 11))
	seen := make(map[string]bool, dailyContentWords)
	words := make([]string, 0, dailyContentWords)
	for len(words) < dailyContentWords {
		var b strings.Builder
		for range 2 + rng.IntN(2) + rng.IntN(2) {
			b.WriteString(dailySyllables[rng.IntN(len(dailySyllables))])
		}
		word := b.String()
		if !seen[word] && !opensWithATerm(word) {
			seen[word] = true
			words = append(words, word)
		}
	}
	return words
}

func opensWithATerm(word string) bool {
	for _, term := range dailyTerms {
		if strings.HasPrefix(strings.ToLower(word), strings.ToLower(term.Word)) {
			return true
		}
	}
	return false
}

// dailyVocabulary samples one language's running text by rank: a handful of
// words in nearly every message, most words in almost none.
type dailyVocabulary struct {
	rng                 *rand.Rand
	function, content   []string
	byFunction, byWords *rand.Zipf
}

// newDailyVocabulary ranks the shared content words in a language's own order,
// so the commonest content word in German mail is not the commonest in English.
func newDailyVocabulary(language string, seed uint64) *dailyVocabulary {
	rng := rand.New(rand.NewPCG(seed, uint64(len(language))+uint64(language[0])))
	content := dailyContentVocabulary()
	rng.Shuffle(len(content), func(i, j int) { content[i], content[j] = content[j], content[i] })
	function := dailyFunctionWords[language]
	return &dailyVocabulary{
		rng: rng, function: function, content: content,
		byFunction: rand.NewZipf(rng, dailyFunctionSkew, 1, uint64(len(function)-1)),
		byWords:    rand.NewZipf(rng, dailyContentSkew, 2, uint64(len(content)-1)),
	}
}

func (v *dailyVocabulary) word() string {
	if v.rng.Float64() < dailyFunctionShare {
		return v.function[v.byFunction.Uint64()]
	}
	return v.content[v.byWords.Uint64()]
}

// sentence is six to sixteen words, with term (when given) at a random place.
func (v *dailyVocabulary) sentence(term string) string {
	words := make([]string, 6+v.rng.IntN(11))
	for i := range words {
		words[i] = v.word()
	}
	if term != "" {
		words[v.rng.IntN(len(words))] = term
	}
	first := []rune(words[0])
	words[0] = strings.ToUpper(string(first[0])) + string(first[1:])
	return strings.Join(words, " ") + "."
}

// dailyParagraphs builds n paragraphs of about a kilobyte from a fixed seed, so
// two runs index the same text and a body's size is the paragraphs it draws.
func dailyParagraphs(language string, n int) []string {
	v := newDailyVocabulary(language, 42)
	paragraphs := make([]string, n)
	for i := range paragraphs {
		var b strings.Builder
		for b.Len() < dailyParagraphBytes {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(v.sentence(""))
		}
		paragraphs[i] = b.String()
	}
	return paragraphs
}

// dailyPlainSubjects are subjects that name no term: two to five words in each
// language in turn.
func dailyPlainSubjects(n int) []string {
	languages := []string{"de", "en", "vi"}
	vocabularies := make([]*dailyVocabulary, len(languages))
	for i, language := range languages {
		vocabularies[i] = newDailyVocabulary(language, 43)
	}
	subjects := make([]string, n)
	for i := range subjects {
		v := vocabularies[i%len(languages)]
		subjects[i] = phrase(v, 2+v.rng.IntN(4))
	}
	return subjects
}

func phrase(v *dailyVocabulary, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = v.word()
	}
	return strings.Join(words, " ")
}

// dailyTermPools gives each term dailyTermPool subjects and sentences that name
// it, term after term in dailyTerms order, so SQL finds term j's at (j-1)*dailyTermPool.
func dailyTermPools() (subjects, sentences []string) {
	for _, term := range dailyTerms {
		v := newDailyVocabulary(term.Language, 44)
		for range dailyTermPool {
			subjects = append(subjects, term.Word+" "+phrase(v, 1+v.rng.IntN(3)))
			sentences = append(sentences, v.sentence(term.Word))
		}
	}
	return subjects, sentences
}

// dailyTermShares is the term table's shares, in its order, for the seeding SQL.
func dailyTermShares() []float64 {
	shares := make([]float64, len(dailyTerms))
	for i, term := range dailyTerms {
		shares[i] = term.Share
	}
	return shares
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

func dailyTokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
}

// dailyBodySample draws paragraphs as the seeding SQL does: any of a language's pool, uniformly.
func dailyBodySample(n int) []string {
	pools := [][]string{
		dailyParagraphs("de", dailyParagraphsPool), dailyParagraphs("en", dailyParagraphsPool),
		dailyParagraphs("vi", dailyParagraphsPool),
	}
	rng := rand.New(rand.NewPCG(1, 2))
	sample := make([]string, n)
	for i := range sample {
		pool := pools[i%len(pools)]
		sample[i] = pool[rng.IntN(len(pool))]
	}
	return sample
}

func TestTheDailyVocabularyIsTheSameEveryRun(t *testing.T) {
	subjects, sentences := dailyTermPools()
	again, againSentences := dailyTermPools()
	if !slices.Equal(dailyParagraphs("vi", 64), dailyParagraphs("vi", 64)) ||
		!slices.Equal(subjects, again) || !slices.Equal(sentences, againSentences) {
		t.Fatal("one seed must give one corpus")
	}
}

func TestGeneratedDailyTextNeverNamesATerm(t *testing.T) {
	text := append(dailyBodySample(6000), dailyPlainSubjects(dailySubjectsPool)...)
	for _, words := range dailyFunctionWords {
		text = append(text, strings.Join(words, " "))
	}
	for _, chunk := range text {
		for _, token := range dailyTokens(chunk) {
			if opensWithATerm(token) || strings.HasPrefix(token, "contr") {
				t.Fatalf("generated text spells %q, so a term's share would no longer be the table's", token)
			}
		}
	}
}

func TestEveryTermPoolEntryNamesItsTerm(t *testing.T) {
	subjects, sentences := dailyTermPools()
	if len(subjects) != len(dailyTerms)*dailyTermPool || len(sentences) != len(subjects) {
		t.Fatalf("pools of %d subjects and %d sentences, want %d each", len(subjects), len(sentences), len(dailyTerms)*dailyTermPool)
	}
	for i := range subjects {
		term := strings.ToLower(dailyTerms[i/dailyTermPool].Word)
		if !slices.Contains(dailyTokens(subjects[i]), term) || !slices.Contains(dailyTokens(sentences[i]), term) {
			t.Fatalf("entry %d (%q / %q) must name %q, the term SQL reads at that place", i, subjects[i], sentences[i], term)
		}
	}
}

// Two thousand bodies of about four paragraphs read like real mail: thousands of
// distinct words, most of them rare, a few function words in nearly every body.
func TestDailyBodiesSpeakAWideVocabularyMostlyOfRareWords(t *testing.T) {
	paragraphs := dailyBodySample(9000)
	inParagraphs := map[string]int{}
	for _, p := range paragraphs {
		seen := map[string]bool{}
		for _, token := range dailyTokens(p) {
			if !seen[token] {
				seen[token] = true
				inParagraphs[token]++
			}
		}
	}
	rare := 0
	for _, n := range inParagraphs {
		if n*200 < len(paragraphs) {
			rare++
		}
	}
	if len(inParagraphs) < 3000 || rare*2 < len(inParagraphs) {
		t.Fatalf("%d distinct words, %d in under 0.5%% of paragraphs; want ≥ 3000, most of them rare", len(inParagraphs), rare)
	}
	if share := float64(inParagraphs["the"]) / float64(len(paragraphs)/3); share < 0.9 {
		t.Fatalf("\"the\" is in %.0f%% of English paragraphs, want nearly all", share*100)
	}
}

// A short prefix matches most mail and a longer one less, as in real text.
func TestDailyPrefixesNarrowAsTheyLengthen(t *testing.T) {
	bodies := dailyBodySample(3000)
	share := func(prefix string) float64 {
		matched := 0
		for _, body := range bodies {
			if slices.ContainsFunc(dailyTokens(body), func(w string) bool { return strings.HasPrefix(w, prefix) }) {
				matched++
			}
		}
		return float64(matched) / float64(len(bodies))
	}
	co, con, cont := share("co"), share("con"), share("cont")
	if co < 0.6 || con >= co || cont >= con || share("contr") != 0 {
		t.Fatalf("co %.2f, con %.2f, cont %.2f, contr %.2f; want co broad, each longer prefix narrower, contr term-only",
			co, con, cont, share("contr"))
	}
	t.Logf("paragraph prefix shares: co %.2f con %.2f cont %.2f", co, con, cont)
}
