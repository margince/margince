// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The scoring half of voice-build evaluation: folding held-out drafts into
// the pinned VoiceProfileEvaluation shape, the activation decision, drift
// against the predecessor inference, and the what-to-add-next guidance.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// The closed vocabularies the evaluation writes: delta classification,
// candidate action, and the sample-draft field names the profile screen
// renders.
const (
	voiceReasonScore          = "score"
	voiceReasonCount          = "count"
	voiceReasonActive         = "active"
	voiceClassRoutine         = "routine"
	voiceClassMaterial        = "material"
	voiceActionAutoActivated  = "auto_activated"
	voiceActionReviewRequired = "review_required"
	voiceDraftFieldSubject    = "subject"
	voiceDraftFieldBody       = "body"
)

// voiceDrift is the candidate-vs-predecessor comparison feeding both the
// material/routine classification and the evaluation record.
type voiceDrift struct {
	identity, signature           float64
	removedAvoid, removedRegister int
	// unreadablePredecessor marks an existing predecessor whose stored
	// inference cannot be decoded: similarity is then unknowable, and an
	// unknowable comparison never auto-activates.
	unreadablePredecessor bool
}

// classify names the drift material when the candidate reads like a
// different contact or drops learned rules; each verdict carries its reason.
func (d voiceDrift) classify(said voiceCopy) (string, []crmcontracts.VoiceReviewReason) {
	classification := voiceClassRoutine
	reasons := []crmcontracts.VoiceReviewReason{}
	if d.unreadablePredecessor {
		classification = voiceClassMaterial
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonUncomparable, Text: said.Say(voiceReviewWords.Uncomparable), Values: map[string]float64{}})
	}
	if d.identity < voiceEvalIdentityFloor {
		classification = voiceClassMaterial
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonIdentityShift, Text: fmt.Sprintf(said.Say(voiceReviewWords.Identity), d.identity), Values: map[string]float64{voiceReasonScore: d.identity}})
	}
	if d.signature < voiceEvalSignatureFloor {
		classification = voiceClassMaterial
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonSignatureShift, Text: fmt.Sprintf(said.Say(voiceReviewWords.Signature), d.signature), Values: map[string]float64{voiceReasonScore: d.signature}})
	}
	if d.removedAvoid > 0 || d.removedRegister > 0 {
		classification = voiceClassMaterial
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonRulesRemoved, Text: fmt.Sprintf(said.Say(voiceReviewWords.Removed), d.removedAvoid, d.removedRegister), Values: map[string]float64{"avoid": float64(d.removedAvoid), "register": float64(d.removedRegister)}})
	}
	return classification, reasons
}

// failureReasons explains exactly which acceptance floor a failed candidate
// missed.
func (said voiceCopy) failureReasons(hardFailures int, structuredValid bool, median float64) []crmcontracts.VoiceReviewReason {
	reasons := []crmcontracts.VoiceReviewReason{}
	if hardFailures > 0 {
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonHardFailures, Text: fmt.Sprintf(said.Say(voiceReviewWords.HardFailures), hardFailures), Values: map[string]float64{voiceReasonCount: float64(hardFailures)}})
	}
	if !structuredValid {
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonMalformed, Text: said.Say(voiceReviewWords.Malformed), Values: map[string]float64{}})
	}
	if median < voiceEvalPassScore {
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonLowScore, Text: fmt.Sprintf(said.Say(voiceReviewWords.BelowFloor), median, voiceEvalPassScore), Values: map[string]float64{voiceReasonScore: median, "floor": voiceEvalPassScore}})
	}
	return reasons
}

// scoreVoiceCandidate folds the drafts into the pinned evaluation shape and
// takes the activation decision.
func (said voiceCopy) scoreVoiceCandidate(artifact ai.VoiceArtifact, drafts []voiceEvalDraft, hardFailures int, structuredValid bool, predecessor *ai.VoiceProfileVersion) voiceEvaluationResult {
	var scores []float64
	for _, draft := range drafts {
		if draft.body != "" {
			scores = append(scores, draft.score)
		}
	}
	median := medianOf(scores)
	drift := voiceDriftAgainst(artifact.Inference, predecessor)
	activeMedian := predecessorMedian(predecessor)
	classification, reasons := drift.classify(said)

	passed := hardFailures == 0 && structuredValid && median >= voiceEvalPassScore
	if activeMedian != nil && median < *activeMedian-voiceEvalRegressionSlack {
		passed = false
		reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonRegression, Text: fmt.Sprintf(said.Say(voiceReviewWords.Regression), median, *activeMedian), Values: map[string]float64{voiceReasonScore: median, voiceReasonActive: *activeMedian}})
	}

	result := voiceEvaluationResult{
		Classification: classification,
		SampleDrafts:   bestSampleDrafts(drafts, 2),
	}
	switch {
	case passed && classification == voiceClassRoutine:
		result.Action = voiceActionAutoActivated
	case passed:
		result.Action = voiceActionReviewRequired
	default:
		result.Action = voiceActionReviewRequired
		result.StatusCode = "quality_regression"
		reasons = append(reasons, said.failureReasons(hardFailures, structuredValid, median)...)
	}
	result.Reasons = reasons
	result.Evaluation = map[string]any{
		"held_out_prompts":             voiceEvalHeldOutPrompts,
		"repeats_per_prompt":           voiceEvalRepeatsPerPrompt,
		"active_median_voice_score":    activeMedian,
		"candidate_median_voice_score": median,
		"anti_ai_hard_failures":        hardFailures,
		"structured_output_valid":      structuredValid,
		"corpus_citations_valid":       true,
		"identity_word_jaccard":        drift.identity,
		"signature_set_jaccard":        drift.signature,
		"removed_avoid_rules":          drift.removedAvoid,
		"removed_register_rules":       drift.removedRegister,
		"classification":               result.Classification,
		"passed":                       passed,
	}
	return result
}

// unevaluatedVoiceResult is the small-corpus outcome: real drift numbers,
// no drafting scores, and an explicit note that evaluation did not run.
func (said voiceCopy) unevaluatedVoiceResult(artifact ai.VoiceArtifact, predecessor *ai.VoiceProfileVersion) voiceEvaluationResult {
	drift := voiceDriftAgainst(artifact.Inference, predecessor)
	classification, reasons := drift.classify(said)
	reasons = append(reasons, crmcontracts.VoiceReviewReason{Code: ai.VoiceReasonTooSmall, Text: said.Say(voiceReviewWords.TooSmall), Values: map[string]float64{}})
	result := voiceEvaluationResult{
		Classification: classification,
		SampleDrafts:   []map[string]any{},
	}
	if predecessor == nil {
		result.Action = voiceActionAutoActivated
	} else {
		result.Action = voiceActionReviewRequired
	}
	result.Reasons = reasons
	result.Evaluation = map[string]any{
		"held_out_prompts":             voiceEvalHeldOutPrompts,
		"repeats_per_prompt":           voiceEvalRepeatsPerPrompt,
		"active_median_voice_score":    predecessorMedian(predecessor),
		"candidate_median_voice_score": nil,
		"anti_ai_hard_failures":        0,
		"structured_output_valid":      true,
		"corpus_citations_valid":       true,
		"identity_word_jaccard":        drift.identity,
		"signature_set_jaccard":        drift.signature,
		"removed_avoid_rules":          drift.removedAvoid,
		"removed_register_rules":       drift.removedRegister,
		"classification":               result.Classification,
		"passed":                       predecessor == nil,
	}
	return result
}

// voiceDriftAgainst compares the candidate inference with the predecessor's
// stored inference. No predecessor → full similarity by definition.
func voiceDriftAgainst(candidate ai.VoiceInference, predecessor *ai.VoiceProfileVersion) voiceDrift {
	prior, ok := predecessorInference(predecessor)
	if !ok {
		// A first build has full similarity by definition; an existing
		// predecessor whose data cannot be read does NOT — that difference
		// decides whether the candidate may auto-activate.
		return voiceDrift{identity: 1, signature: 1, unreadablePredecessor: predecessor != nil}
	}
	return voiceDrift{
		identity:        jaccard(wordSet(append(candidate.Vocabulary, candidate.IdentitySummary)), wordSet(append(prior.Vocabulary, prior.IdentitySummary))),
		signature:       jaccard(moveSet(candidate.SignatureMoves), moveSet(prior.SignatureMoves)),
		removedAvoid:    missingCount(prior.Avoid, candidate.Avoid),
		removedRegister: missingCount(prior.RegisterNotes, candidate.RegisterNotes),
	}
}

func predecessorInference(predecessor *ai.VoiceProfileVersion) (ai.VoiceInference, bool) {
	if predecessor == nil {
		return ai.VoiceInference{}, false
	}
	raw, ok := predecessor.ProfileJSON["inference"]
	if !ok {
		return ai.VoiceInference{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return ai.VoiceInference{}, false
	}
	var inference ai.VoiceInference
	if err := json.Unmarshal(encoded, &inference); err != nil {
		return ai.VoiceInference{}, false
	}
	return inference, true
}

func predecessorMedian(predecessor *ai.VoiceProfileVersion) *float64 {
	if predecessor == nil {
		return nil
	}
	value, ok := predecessor.Evaluation["candidate_median_voice_score"]
	if !ok {
		return nil
	}
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	return &number
}

func bestSampleDrafts(drafts []voiceEvalDraft, limit int) []map[string]any {
	ordered := append([]voiceEvalDraft(nil), drafts...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].score > ordered[j].score })
	// Concrete empty, never null: the profile screen renders this list as-is.
	out := make([]map[string]any, 0, limit)
	for _, draft := range ordered {
		if draft.body == "" || len(out) == limit {
			continue
		}
		out = append(out, map[string]any{
			"prompt":               draft.prompt,
			voiceDraftFieldSubject: draft.subject,
			voiceDraftFieldBody:    draft.body,
			"voice_score":          draft.score,
		})
	}
	return out
}

// voiceGuidance derives the what-to-add-next nudge from the corpus meter and
// register coverage — deterministic, so the UI can trust it verbatim.
func voiceGuidance(stats ai.VoiceStats) map[string]any {
	gaps := make([]string, 0, 2)
	if stats.RegisterWords["spoken"] == 0 {
		gaps = append(gaps, "spoken")
	}
	if stats.RegisterWords["email"] == 0 {
		gaps = append(gaps, "email")
	}
	next := ""
	key := ""
	words := 0
	switch {
	case len(gaps) > 0 && gaps[0] == "spoken":
		key = "add_transcript"
		next = "Add a call or meeting transcript — spoken words are your highest-signal source."
	case len(gaps) > 0:
		key = "add_email"
		next = "Add sent emails — they are the primary source for how you write at work."
	case stats.WordCount < ai.CorpusTargetWords:
		key = "add_words"
		words = ai.CorpusTargetWords - stats.WordCount
		next = fmt.Sprintf("Add roughly %d more words to reach the sharp band.", words)
	default:
		key = "at_target"
		next = "Your corpus is at target; keep it fresh by adding recent writing occasionally."
	}
	// next_best_key + next_best_words let the UI localize the nudge; the
	// English prose stays as the honest fallback for older clients.
	return map[string]any{"next_best": next, "next_best_key": key, "next_best_words": words, "register_gaps": gaps}
}

func wordSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		for word := range strings.FieldsSeq(strings.ToLower(value)) {
			trimmed := strings.Trim(word, ".,;:!?\"'()")
			if len([]rune(trimmed)) >= 4 {
				set[trimmed] = true
			}
		}
	}
	return set
}

func moveSet(moves []ai.VoiceSignatureMove) map[string]bool {
	set := map[string]bool{}
	for _, move := range moves {
		key := strings.Join(strings.Fields(strings.ToLower(move.Move)), " ")
		if key != "" {
			set[key] = true
		}
	}
	return set
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	intersection := 0
	for key := range a {
		if b[key] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 1
	}
	return round4(float64(intersection) / float64(union))
}

func missingCount(prior, current []string) int {
	have := map[string]bool{}
	for _, value := range current {
		have[strings.Join(strings.Fields(strings.ToLower(value)), " ")] = true
	}
	missing := 0
	for _, value := range prior {
		if !have[strings.Join(strings.Fields(strings.ToLower(value)), " ")] {
			missing++
		}
	}
	return missing
}

func medianOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return round4((sorted[middle-1] + sorted[middle]) / 2)
	}
	return sorted[middle]
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func round4(value float64) float64 { return math.Round(value*10000) / 10000 }
