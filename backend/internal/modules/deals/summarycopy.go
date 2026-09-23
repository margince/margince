// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The inbox line the follow-up pass writes when it stages a proposal.
//
// An approval's summary is shared-record text: stored once and read by every
// seat, so it follows the installation's base language, resolved when the
// proposal is staged. Rows already stored keep the words they were written in.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// summaryCopy is one language's set. Every field is required, and the census
// (summarycopy_test.go) refuses a partial set.
type summaryCopy struct {
	// draftFollowUp asks for a follow-up on a touched deal. %q is the deal's
	// name, then %s the interaction kind (an identifier, untranslated) and %s
	// its date, in that order.
	draftFollowUp string

	// The stage policy's reasons, one per decision it can reach. A field whose
	// name ends in Key has one %s, the criterion's key, an identifier; the rest
	// have no hole.
	stageAutoApply        string
	stageAllSettled       string
	stageUnreliable       string
	stageNoCriteria       string
	stageStillAsksForKey  string
	stageOurSideOnlyKey   string
	stageClosing          string
	stageCrossPipeline    string
	stageSkips            string
	stageOptionalOpenKey  string
	stageNotAgreedKey     string
	stageUncertainKey     string
	protectedMovedByYou   string
	protectedUndoneBefore string
	protectedTurnedDown   string
}

// summaryByLang is the census, keyed by textlang.Lang so the test can walk
// textlang.Shipped and ask this map directly.
var summaryByLang = map[textlang.Lang]summaryCopy{
	textlang.English: {
		draftFollowUp: "Draft a follow-up on %q — a %s on %s left no next step planned",

		stageAutoApply:        "every exit criterion is settled by the other side's own words, and this transition has been measured long enough to move itself",
		stageAllSettled:       "every exit criterion for this stage is settled",
		stageUnreliable:       "the evidence for this stage cannot be relied on as it stands",
		stageNoCriteria:       "this stage does not say what it takes to leave it, so nothing here can settle it",
		stageStillAsksForKey:  "the stage still asks for %s, and nothing says it is settled",
		stageOurSideOnlyKey:   "%s names something the buyer does, and only our own side has said it",
		stageClosing:          "this move enters or leaves a closing stage, which is always a contact's call",
		stageCrossPipeline:    "this move would cross into another pipeline",
		stageSkips:            "this move skips or goes back a stage rather than advancing by one",
		stageOptionalOpenKey:  "%s is not settled, and the stage lists it",
		stageNotAgreedKey:     "%s does not rest on anything the other side agreed to",
		stageUncertainKey:     "the reading of %s is not certain enough to act on unasked",
		protectedMovedByYou:   "you moved this deal yourself in the last fortnight",
		protectedUndoneBefore: "a stage move on this deal was undone before",
		protectedTurnedDown:   "you turned this move down, and nothing new has been learned since",
	},
	textlang.German: {
		draftFollowUp: "Entwirf ein Follow-up zu %q: Nach dem Austausch (%s) am %s ist kein nächster Schritt geplant.",

		stageAutoApply:        "jedes Austrittskriterium ist durch die eigenen Worte der Gegenseite erfüllt, und dieser Übergang wurde lange genug gemessen, um den Deal selbst zu verschieben",
		stageAllSettled:       "jedes Austrittskriterium dieser Phase ist erfüllt",
		stageUnreliable:       "auf die Belege für diese Phase ist in ihrem jetzigen Stand kein Verlass",
		stageNoCriteria:       "diese Phase legt nicht fest, was es braucht, um sie zu verlassen, also kann hier nichts sie erfüllen",
		stageStillAsksForKey:  "die Phase verlangt noch %s, und nichts zeigt, dass es erfüllt ist",
		stageOurSideOnlyKey:   "%s beschreibt etwas, das der Käufer tut, und nur unsere eigene Seite hat es gesagt",
		stageClosing:          "dieser Schritt führt in eine Abschlussphase oder aus ihr heraus, und darüber entscheidet immer ein Mensch",
		stageCrossPipeline:    "dieser Schritt würde in eine andere Pipeline wechseln",
		stageSkips:            "dieser Schritt überspringt eine Phase oder geht eine zurück, statt um eine voranzugehen",
		stageOptionalOpenKey:  "%s ist nicht erfüllt, und die Phase führt es auf",
		stageNotAgreedKey:     "%s beruht auf nichts, dem die Gegenseite zugestimmt hat",
		stageUncertainKey:     "die Lesart von %s ist nicht sicher genug, um ungefragt zu handeln",
		protectedMovedByYou:   "du hast diesen Deal in den letzten zwei Wochen selbst verschoben",
		protectedUndoneBefore: "ein Phasenwechsel bei diesem Deal wurde schon einmal rückgängig gemacht",
		protectedTurnedDown:   "du hast diesen Schritt abgelehnt, und seitdem ist nichts Neues bekannt geworden",
	},
	textlang.Vietnamese: {
		draftFollowUp: "Soạn một follow-up cho %q: sau tương tác (%s) vào ngày %s chưa có bước tiếp theo nào được lên kế hoạch.",

		stageAutoApply:        "mọi tiêu chí rời giai đoạn đều đã được xác nhận bằng chính lời của phía bên kia, và bước chuyển này đã được đo lường đủ lâu để tự thực hiện",
		stageAllSettled:       "mọi tiêu chí rời giai đoạn này đều đã được đáp ứng",
		stageUnreliable:       "bằng chứng cho giai đoạn này, ở hiện trạng, chưa thể tin cậy được",
		stageNoCriteria:       "giai đoạn này không nêu điều kiện để rời khỏi nó, nên không có gì ở đây có thể đáp ứng được",
		stageStillAsksForKey:  "giai đoạn vẫn yêu cầu %s, và không có gì cho thấy điều đó đã được đáp ứng",
		stageOurSideOnlyKey:   "%s là việc bên mua phải làm, nhưng chỉ phía chúng ta nói điều đó",
		stageClosing:          "bước này đi vào hoặc rời khỏi một giai đoạn chốt, việc này luôn do con người quyết định",
		stageCrossPipeline:    "bước này sẽ chuyển sang một pipeline khác",
		stageSkips:            "bước này bỏ qua hoặc lùi lại một giai đoạn thay vì tiến thêm một bước",
		stageOptionalOpenKey:  "%s chưa được đáp ứng, và giai đoạn có liệt kê tiêu chí này",
		stageNotAgreedKey:     "%s không dựa trên bất cứ điều gì phía bên kia đã đồng ý",
		stageUncertainKey:     "cách đọc %s chưa đủ chắc chắn để hành động mà không hỏi",
		protectedMovedByYou:   "bạn đã tự chuyển deal này trong hai tuần qua",
		protectedUndoneBefore: "một lần chuyển giai đoạn của deal này đã từng bị hoàn tác",
		protectedTurnedDown:   "bạn đã từ chối bước này, và từ đó đến nay chưa có thông tin mới nào",
	},
}

// summaryIn answers the set for the installation's base language, and English
// for a language this table has not learned: the language comes off a settings
// row, and a sentence in English beats no sentence.
func summaryIn(ctx context.Context, language baselanguage.Resolver) summaryCopy {
	return summaryFor(language.Resolve(ctx))
}

// summaryFor is summaryIn for a caller that already holds the language.
func summaryFor(lang textlang.Lang) summaryCopy {
	if said, ok := summaryByLang[lang]; ok {
		return said
	}
	return summaryByLang[textlang.English]
}

// WithBaseLanguage returns a copy that writes its staged summaries in the
// installation's base language. A reconciler built without one writes English.
func (r *FollowUpReconciler) WithBaseLanguage(language baselanguage.Resolver) *FollowUpReconciler {
	c := *r
	c.language = language
	return &c
}
