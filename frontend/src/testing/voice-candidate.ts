import type { components } from "../api/schema";

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

export function candidateVersion(
  reviewReasons: readonly string[],
): VoiceProfileVersion {
  return {
    id: "v-1",
    profile_id: "p-1",
    profile_version: 1,
    status: "candidate",
    voice_profile_md: "# Voice DNA",
    profile_json: {
      inference: {
        identity_summary: "Direct, concrete, few adjectives.",
        thinking_pattern: "State the ask, then the reason, then the deadline.",
        signature_moves: [{ quote: "Passt das so?", source_id: "s1" }],
        avoid: ["corporate filler"],
      },
      sample_drafts: [{ scenario: "Follow-up", draft: "Moin Stefan, kurz..." }],
      guidance: { next_best: "Add more sent mail." },
    },
    stats_json: { corpus_words: 1200, source_count: 3 },
    source_hash: "h",
    source_count: 3,
    reason: "manual",
    predecessor_version: null,
    activation_policy_version: "2",
    model_provider: "routed",
    model_name: "claude-haiku-4.5",
    builder_version: "voicebuilder/1",
    source: "build",
    captured_by: "human:x",
    evaluation: {
      held_out_prompts: 5,
      repeats_per_prompt: 3,
      active_median_voice_score: null,
      candidate_median_voice_score: 0.56,
      anti_ai_hard_failures: 0,
      structured_output_valid: false,
      corpus_citations_valid: true,
      identity_word_jaccard: 1,
      signature_set_jaccard: 1,
      removed_avoid_rules: 0,
      removed_register_rules: 0,
      classification: "material",
      passed: false,
    },
    review_reasons: [...reviewReasons],
    version: 1,
    created_at: "2026-08-30T03:42:00Z",
    updated_at: "2026-08-30T03:42:00Z",
    archived_at: null,
    activated_at: null,
  };
}
