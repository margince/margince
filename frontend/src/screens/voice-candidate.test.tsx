/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ActiveVoiceInsights } from "./voice-versions";

// The banner asks the owner to accept or reject a voice that will write their
// mail. Everything that decision needs has to be ON it: what the voice says
// about them, and why it is waiting rather than activating itself. It shipped
// with a title, the evaluator's own log sentences, and two buttons — so "Use
// this version" meant approving writing nobody had seen, and reading it first
// was impossible.

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

function candidateVersion(
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

function stub(version: VoiceProfileVersion) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [version],
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
}

function view(ui: ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("reviewing a candidate voice", () => {
  it("shows the voice itself, so the decision is about something visible", async () => {
    stub(candidateVersion([]));
    view(
      <ActiveVoiceInsights
        profileId="p-1"
        canEdit
        onChanged={() => undefined}
      />,
    );

    // What the build actually learned, on the card that asks about it.
    expect(
      await screen.findByText(
        "State the ask, then the reason, then the deadline.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/Direct, concrete/)).toBeTruthy();
    // And the fact that nothing is written in it yet.
    expect(screen.getByText(/It is not in use yet/)).toBeTruthy();
  });

  // The evaluator writes for an operator reading a log. "median voice score
  // 0.56 is below the 0.60 floor" is not a sentence the person being asked to
  // decide can act on.
  it("says why it is waiting in words about the owner's own voice", async () => {
    stub(
      candidateVersion([
        "the model returned malformed drafts during evaluation",
        "median voice score 0.56 is below the 0.60 floor",
      ]),
    );
    view(
      <ActiveVoiceInsights
        profileId="p-1"
        canEdit
        onChanged={() => undefined}
      />,
    );

    expect(
      await screen.findByText(/scored 0.56 against your own writing/),
    ).toBeTruthy();
    expect(
      screen.getByText(/could not read some of the sample drafts/),
    ).toBeTruthy();
    // The raw operator sentences are gone from the reader's view.
    expect(screen.queryByText(/below the 0.60 floor/)).toBeNull();
    // And the reader is told what each answer means for them.
    expect(screen.getByText(/If it reads like you, use it/)).toBeTruthy();
  });

  // A reason nobody can read is bad; a reason nobody can SEE is worse, because
  // the candidate would then be held back for a cause never stated.
  it("shows a reason it does not recognize rather than dropping it", async () => {
    stub(candidateVersion(["some future evaluator sentence"]));
    view(
      <ActiveVoiceInsights
        profileId="p-1"
        canEdit
        onChanged={() => undefined}
      />,
    );

    expect(
      await screen.findByText("some future evaluator sentence"),
    ).toBeTruthy();
  });
});
