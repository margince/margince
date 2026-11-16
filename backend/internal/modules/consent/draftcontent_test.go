// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"slices"
	"testing"
)

var vietnameseClaims = []string{"nhất", "duy nhất", "tốt nhất", "số một"}

const vietnameseOffer = "Kính gửi quý khách,\n\nChúng tôi xin giới thiệu dịch vụ của chúng tôi cho doanh nghiệp của quý khách."

func findingOf(findings []DraftFinding, requirement string) (DraftFinding, bool) {
	for _, f := range findings {
		if f.Requirement == requirement {
			return f, true
		}
	}
	return DraftFinding{}, false
}

func TestADraftClaimingTheBestWithoutAProofIsFlagged(t *testing.T) {
	body := vietnameseOffer + "\n\nĐây là dịch vụ tốt nhất cho quý khách."
	f, ok := findingOf(draftFindings("vi", vietnameseClaims, "[QC] Ưu đãi", body), DraftClaimWithoutProof)
	if !ok {
		t.Fatal("a draft claiming \"tốt nhất\" with no proof reference was not flagged")
	}
	if !slices.Equal(f.Words, []string{"tốt nhất"}) {
		t.Errorf("the finding names %v, want the claim as written and not its parts", f.Words)
	}
}

func TestAClaimBesideAStatedProofIsNotFlagged(t *testing.T) {
	body := vietnameseOffer + "\n\nĐây là dịch vụ tốt nhất cho quý khách.\n" +
		"Theo Giấy chứng nhận số 123/2026/GCN-ABC ngày 01/02/2026."
	if _, ok := findingOf(draftFindings("vi", vietnameseClaims, "", body), DraftClaimWithoutProof); ok {
		t.Error("a claim with its document, number and date beside it was flagged")
	}
	// A date alone names no document.
	dateOnly := vietnameseOffer + "\n\nTốt nhất từ 01/02/2026."
	if _, ok := findingOf(draftFindings("vi", vietnameseClaims, "", dateOnly), DraftClaimWithoutProof); !ok {
		t.Error("a bare date was taken for a proof reference")
	}
}

// A claim word inside a longer word is not the claim.
func TestAClaimMatchesWholeWordsOnly(t *testing.T) {
	if got := claimsIn("Sản phẩm nhấtquán", vietnameseClaims); len(got) != 0 {
		t.Errorf("claims %v found inside a longer word", got)
	}
	if got := claimsIn("SỐ MỘT thị trường", vietnameseClaims); !slices.Equal(got, []string{"số một"}) {
		t.Errorf("claims %v, want the upper-case claim found", got)
	}
}

func TestADraftThatOpensInAnotherLanguageIsFlagged(t *testing.T) {
	english := "Dear customer,\n\nWe would like to introduce our new service to your company and its team.\n\n" + vietnameseOffer
	if _, ok := findingOf(draftFindings("vi", nil, "", english), DraftLanguageFirst); !ok {
		t.Error("a draft opening in English was not flagged for a pack asking for Vietnamese first")
	}
	vietnameseFirst := vietnameseOffer + "\n\nDear customer, we would like to introduce our new service to your company."
	if _, ok := findingOf(draftFindings("vi", nil, "", vietnameseFirst), DraftLanguageFirst); ok {
		t.Error("a draft opening in Vietnamese was flagged")
	}
	if len(draftFindings("", nil, "", english)) != 0 {
		t.Error("a pack with no content rules flagged a draft")
	}
}

// A digit continues a word, as in the draft checks: "số 1" is not "số 100".
func TestANumberedClaimDoesNotMatchALongerNumber(t *testing.T) {
	if got := claimsIn("Chúng tôi phục vụ số 100 khách hàng.", []string{"số 1"}); len(got) != 0 {
		t.Errorf("claims %v found inside a longer number", got)
	}
	if got := claimsIn("Chúng tôi là số 1.", []string{"số 1"}); !slices.Equal(got, []string{"số 1"}) {
		t.Errorf("claims %v, want the numbered claim found", got)
	}
}

// Every use of a longer claim is taken out before a shorter one is sought.
func TestALongerClaimUsedTwiceHidesItsShorterPart(t *testing.T) {
	got := claimsIn("Tốt nhất cho bạn. Giá tốt nhất.", vietnameseClaims)
	if !slices.Equal(got, []string{"tốt nhất"}) {
		t.Errorf("claims %v, want only \"tốt nhất\"", got)
	}
}
