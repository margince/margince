// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { VOICE_MIN_WORDS } from "../voice-intake-core";
import type { ConversationQuestion } from "./conversation-types";
import type { CorpusManifestEntry } from "./use-voice-corpus";
import {
  VoiceBuildScene,
  VoiceCollectScene,
  VoiceResultScene,
  VoiceSpeakerScene,
} from "./voice-scenes";

// The voice act's four work surfaces, one scene at a time: collect the writing,
// say who is speaking, watch the model learn it, read what it learned.
//
// Every figure on these scenes is the SERVER's count — words kept, sources
// ingested, mean sentence length — so the stories hand those counts in as props
// rather than deriving them, which is the only way the notation on screen is the
// notation the wire produced. `FloorCleared` and the German pair carry four
// digits because that is the narrowest number at which de-DE grouping is visible
// at all: below it a grouped and an ungrouped figure are the same characters.
//
// The build scene's ring is the one number that is deliberately NOT in the
// accessibility tree — the stage checklist beside it and the rail's own log
// already carry the progress in words, and announcing crawling digits on every
// tick would talk over both.

type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

// `maturity` and `quality_band` are the server's own readings of the corpus, so
// they are derived from the count rather than picked per story: a fixture that
// set them independently would describe a summary no server sends.
function summary(totalWords: number, sources: number): CorpusSummary {
  return {
    total_words: totalWords,
    target_words: 30_000,
    maturity: totalWords < VOICE_MIN_WORDS ? "collecting" : "provisional",
    quality_band: totalWords < VOICE_MIN_WORDS ? "thin" : "good",
    source_count: sources,
    register_words: { email: totalWords },
  };
}

const MANIFEST: readonly CorpusManifestEntry[] = [
  {
    ref: "src-1",
    label: "Sent mail, last quarter.mbox",
    keptWords: 2840,
    inputWords: 2840,
    transcript: false,
    lines: [
      "Let us move the kickoff to Thursday so the data team can actually join.",
      "I have attached the revised offer with the two changes we discussed on the call.",
      "Short answer: yes, but only if the migration finishes before the quarter closes.",
      "Happy to walk your finance team through the pricing whenever suits them.",
    ],
  },
  {
    ref: "src-2",
    label: "Kickoff call, Brandt Automotive.vtt",
    keptWords: 1380,
    inputWords: 4210,
    transcript: true,
    lines: [],
  },
];

// The scene owns the file input, so the ref has to come from a real render
// rather than a module-level object: a detached ref never receives the node, and
// Browse would then silently do nothing in the one story about pressing it.
function Collect({
  corpus,
  manifest = [],
  canBuild = false,
  startPending = false,
  startError = null,
  locale,
}: Readonly<{
  corpus: CorpusSummary | null;
  manifest?: readonly CorpusManifestEntry[];
  canBuild?: boolean;
  startPending?: boolean;
  startError?: string | null;
  locale?: "de";
}>) {
  const fileRef = useRef<HTMLInputElement | null>(null);
  return (
    <StoryProviders locale={locale}>
      <VoiceCollectScene
        summary={corpus}
        manifest={manifest}
        fileRef={fileRef}
        onFiles={() => {}}
        onAddPaste={() => {}}
        onBuild={() => {}}
        onSkip={() => {}}
        canBuild={canBuild}
        startPending={startPending}
        startError={startError}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof VoiceCollectScene> = {
  title: "Onboarding/Conversation/Voice scenes",
  component: VoiceCollectScene,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof VoiceCollectScene>;

/** Nothing handed over yet: the drop target, and a meter with nothing in it. */
export const CollectEmpty: Story = { render: () => <Collect corpus={null} /> };

/**
 * Short of the floor. The meter says how far off it is rather than only refusing
 * — a Build verb that is inert with no figure beside it leaves the reader
 * guessing how much more to paste.
 */
export const CollectBelowFloor: Story = {
  render: () => (
    <Collect
      corpus={summary(320, 1)}
      manifest={[MANIFEST[0]]}
      canBuild={false}
    />
  ),
};

/**
 * Past the floor, with a source whose transcript filtering discarded turns —
 * kept-of-total is shown only there, because on a plain document the two counts
 * are the same number written twice.
 */
export const CollectFloorCleared: Story = {
  render: () => (
    <Collect corpus={summary(4220, 2)} manifest={MANIFEST} canBuild={true} />
  ),
};

/** The same cleared meter in German — the notation the reader writes numbers in,
 *  on the count and on the floor it is measured against. */
export const CollectFloorClearedGerman: Story = {
  render: () => (
    <Collect
      corpus={summary(4220, 2)}
      manifest={MANIFEST}
      canBuild={true}
      locale="de"
    />
  ),
};

/** The build request in flight: the verb is held, and the meter does NOT dim —
 *  a corpus that already cleared the floor has lost no ground. */
export const CollectStarting: Story = {
  render: () => (
    <Collect
      corpus={summary(4220, 2)}
      manifest={MANIFEST}
      canBuild={true}
      startPending={true}
    />
  ),
};

/** The build refused to start. The reason is the server's sentence, and the
 *  corpus it was refused against is still on screen to try again from. */
export const CollectStartRefused: Story = {
  render: () => (
    <Collect
      corpus={summary(4220, 2)}
      manifest={MANIFEST}
      canBuild={true}
      startError="A build is already running for this installation."
    />
  ),
};

/** At 390px the drop acts, the meter and the foot verbs stack, and the pinned
 *  primary has to stay reachable under them. */
export const CollectPhone: Story = {
  tags: ["uat-phone"],
  render: () => (
    <Collect corpus={summary(4220, 2)} manifest={MANIFEST} canBuild={true} />
  ),
};

const SPEAKER_QUESTION: ConversationQuestion = {
  id: "speaker:src-2",
  i18nKey: "ob.conv.voice.speakerQuestion",
  options: [
    {
      value: "SPEAKER_00",
      label: "Speaker 1",
      detailKey: "ob.conv.voice.speakerOptionDetail",
      params: { words: "2,140", turns: "38" },
    },
    {
      value: "SPEAKER_01",
      label: "Speaker 2",
      detailKey: "ob.conv.voice.speakerOptionDetail",
      params: { words: "1,380", turns: "24" },
    },
  ],
};

/**
 * Which voice in the transcript is the reader's own. Continue stays held until
 * one is picked: a speaker chosen by default would file somebody else's words as
 * this company's writing, and nothing downstream would ever say so.
 */
export const SpeakerAsk: Story = {
  render: () => (
    <StoryProviders>
      <VoiceSpeakerScene question={SPEAKER_QUESTION} onAnswer={() => {}} />
    </StoryProviders>
  ),
};

/** The same decision in German, where the option detail and the foot line both
 *  run longer than the card was first drawn for. */
export const SpeakerAskGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <VoiceSpeakerScene question={SPEAKER_QUESTION} onAnswer={() => {}} />
    </StoryProviders>
  ),
};

/** A queued build: no stage reported yet, so the ring claims nothing and the
 *  checklist has nothing done. */
export const BuildQueued: Story = {
  render: () => (
    <StoryProviders>
      <VoiceBuildScene stage={null} summary={summary(4220, 2)} sources={2} />
    </StoryProviders>
  ),
};

/** Mid-pipeline. The ceiling is derived from the stage the server reported, and
 *  the ring crawls toward it rather than jumping. */
export const BuildExtracting: Story = {
  render: () => (
    <StoryProviders>
      <VoiceBuildScene stage="extract" summary={summary(4220, 2)} sources={2} />
    </StoryProviders>
  ),
};

/** The last stage. The ring still stops short of full — a build reaches 100 only
 *  by finishing, at which point this is no longer the scene on screen. */
export const BuildActivating: Story = {
  render: () => (
    <StoryProviders>
      <VoiceBuildScene
        stage="activate"
        summary={summary(4220, 2)}
        sources={2}
      />
    </StoryProviders>
  ),
};

/** The build in German: the word and source counts in the reader's notation,
 *  beside a percentage that is decorative in every language. */
export const BuildExtractingGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <VoiceBuildScene stage="extract" summary={summary(4220, 2)} sources={2} />
    </StoryProviders>
  ),
};

function version(
  overrides: Partial<VoiceProfileVersion> = {},
): VoiceProfileVersion {
  return {
    id: "vpv-1",
    profile_id: "vp-1",
    profile_version: 1,
    status: "active",
    voice_profile_md: "# Voice\n\nPlain, specific, quick to the ask.",
    profile_json: {
      inference: {
        identity_summary: "Plain, specific, and quick to the ask.",
        thinking_pattern: "Leads with the constraint, then the offer.",
        observed_obsessions: ["delivery dates", "who signs"],
        signature_moves: [
          {
            move: "Names the blocker before the ask",
            quote: "Depot's offline the 14th — can we sign before then?",
          },
          { move: "Closes on one question", quote: "Does Thursday work?" },
        ],
        avoid: ["exclamation marks", "hedging"],
      },
      guidance: {
        next_best: "Add three sent threads from the last quarter.",
        next_best_key: "recent_sent",
        next_best_words: 900,
      },
      sample_drafts: [
        {
          subject: "Retrofit timeline",
          body: "Lars — the depot window moved to the 14th. Can you confirm the quote before then?",
          score: 0.82,
        },
        {
          subject: "Quote follow-up",
          body: "Still holding the March slot for you. Say the word and I'll book it.",
          score: 0.77,
        },
      ],
    },
    stats_json: {
      word_count: 4220,
      mean_sentence_words: 14,
      sample_count: 2,
    },
    source_hash: "sha256:9f21",
    source_count: 2,
    reason: "onboarding",
    predecessor_version: null,
    model_provider: "deepseek",
    model_name: "deepseek-chat",
    builder_version: "voice-builder/3",
    source: "onboarding",
    captured_by: "u-me",
    activation_policy_version: "policy/2",
    evaluation: {
      held_out_prompts: 5,
      repeats_per_prompt: 3,
      active_median_voice_score: null,
      candidate_median_voice_score: 0.81,
      anti_ai_hard_failures: 0,
      structured_output_valid: true,
      corpus_citations_valid: true,
      identity_word_jaccard: 0.74,
      signature_set_jaccard: 0.66,
      removed_avoid_rules: 0,
      removed_register_rules: 0,
      classification: "routine",
      passed: true,
    },
    review_reasons: [],
    version: 1,
    created_at: "2026-08-25T09:00:00Z",
    updated_at: "2026-08-25T09:04:00Z",
    archived_at: null,
    activated_at: "2026-08-25T09:04:00Z",
    ...overrides,
  };
}

function result(
  built: VoiceProfileVersion | null,
  loading = false,
  locale?: "de",
) {
  return () => (
    <StoryProviders locale={locale}>
      <VoiceResultScene
        loading={loading}
        version={built}
        onContinue={() => {}}
        onRevise={() => {}}
      />
    </StoryProviders>
  );
}

/** Reading the profile the build produced. */
export const ResultLoading: Story = { render: result(null, true) };

/**
 * The build finished and the profile did not come back. The scene says that
 * plainly rather than drawing an empty board: a reader who cannot see what was
 * learned must not be shown a shape implying nothing was.
 */
export const ResultAbsent: Story = { render: result(null) };

/** What the build learned: the sample it would send, and the reading behind it.
 *  Two samples, so the cycle control has somewhere to go. */
export const ResultRich: Story = { render: result(version()) };

/** The same board in German. */
export const ResultRichGerman: Story = {
  render: result(version(), false, "de"),
};

/**
 * A starter corpus: too few sources to hold any back, so the build reserved no
 * held-out sample and there is no draft to read. The board goes single-column
 * and the lead copy changes — never a sample card with an invented draft in it.
 */
export const ResultWithoutSample: Story = {
  render: result(
    version({
      profile_json: {
        inference: {
          identity_summary: "Short lines, one ask at a time.",
          signature_moves: [],
          avoid: [],
        },
        guidance: {},
        sample_drafts: [],
      },
      stats_json: { word_count: 860, mean_sentence_words: 9, sample_count: 1 },
    }),
  ),
};

/**
 * A candidate rather than an active profile: the evaluation wants a human before
 * anything writes in this voice, and the scene says so instead of letting the
 * reader believe it is already in use.
 */
export const ResultCandidate: Story = {
  render: result(version({ status: "candidate" })),
};

/** At 390px the two columns become one and the sample body cannot share a line
 *  with its own labels. */
export const ResultPhone: Story = {
  tags: ["uat-phone"],
  render: result(version()),
};
