// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// firstPassageRef is the ref the prompt shows for the first passage retrieved.
const firstPassageRef = "p1"

func TestTheCorpusPromptNamesPassagesByRefAndAClaimReadsBack(t *testing.T) {
	passages := askPassages()
	req := CorpusAskRequest("how long are messages kept", passages, string(textlang.English))
	if strings.Contains(req.Messages[0].Content, passages[0].ChunkID.String()) {
		t.Fatalf("the prompt shows a passage's chunk id:\n%s", req.Messages[0].Content)
	}
	answer, err := GroundCorpusAnswer(corpusReply(askedClaim{
		Text: "Messages are kept for 400 days.", ID: firstPassageRef, Quote: "kept for 400 days",
	}), passages)
	if err != nil || len(answer.Claims) != 1 || answer.Claims[0].ChunkId.String() != passages[0].ChunkID.String() {
		t.Fatalf("answer = %+v, err %v; want one claim on the first passage", answer, err)
	}
}

func TestACorpusClaimCitingARefNeverShownOrSpellingOneIsDropped(t *testing.T) {
	passages := askPassages()
	for name, claim := range map[string]askedClaim{
		"a ref never shown":  {Text: "Messages are kept for 400 days.", ID: "p99", Quote: "kept for 400 days"},
		"the raw chunk id":   {Text: "Messages are kept for 400 days.", ID: passages[0].ChunkID.String(), Quote: "kept for 400 days"},
		"a ref in the prose": {Text: "Per p1, messages are kept for 400 days.", ID: firstPassageRef, Quote: "kept for 400 days"},
	} {
		answer, err := GroundCorpusAnswer(corpusReply(claim), passages)
		if err != nil || len(answer.Claims) != 0 {
			t.Errorf("%s: kept %d claims, err %v; want the claim dropped", name, len(answer.Claims), err)
		}
	}
}
