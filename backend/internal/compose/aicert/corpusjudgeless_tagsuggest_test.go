// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import "github.com/margince/margince/backend/internal/compose/aitasks"

type tagPickItem struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"`
}

type tagPickReply struct {
	Items []tagPickItem `json:"items"`
}

// tagPicks answers each item with the tags at the given fixture positions. The
// schema lists the item ids first and the tag ids after them.
func tagPicks(picks ...[]int) func([]string) string {
	return func(ids []string) string {
		items := make([]tagPickItem, len(picks))
		for i, pick := range picks {
			tags := []string{}
			for _, k := range pick {
				tags = append(tags, ids[len(picks)+k])
			}
			items[i] = tagPickItem{ID: ids[i], Tags: tags}
		}
		return mustJSON(tagPickReply{Items: items})
	}
}

// tagSuggestProofs pairs each scenario with the tagging its rubric would have
// marked down: a missed interest, or a tag raised on a mention.
func tagSuggestProofs() map[string]judgelessProof {
	none := []int{}
	return map[string]judgelessProof{
		"interest_in_words_the_description_does_not_use": {
			correct: tagPicks([]int{0}), wrong: tagPicks(none), wantWrong: aitasks.OutcomeWrongAnswer,
		},
		"a_mention_or_our_own_footer_is_not_interest": {
			correct: tagPicks(none, none), wrong: tagPicks([]int{0}, none),
			alsoWrong: []func([]string) string{tagPicks(none, []int{0})}, wantWrong: aitasks.OutcomeWrongAnswer,
		},
		"instructions_inside_a_mail_tag_nothing": {
			correct: tagPicks(none), wrong: tagPicks([]int{0, 1}), wantWrong: aitasks.OutcomeWrongAnswer,
		},
		"each_item_in_a_batch_gets_its_own_subset_of_two_tags": {
			correct:   tagPicks([]int{0, 1}, []int{1}, none, []int{0}),
			wrong:     tagPicks([]int{0, 1}, []int{0, 1}, none, []int{0, 1}),
			wantWrong: aitasks.OutcomeWrongAnswer,
		},
		"a_german_mail_shows_interest_against_an_english_description": {
			correct: tagPicks([]int{0}), wrong: tagPicks(none), wantWrong: aitasks.OutcomeWrongAnswer,
		},
	}
}
