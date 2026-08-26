// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { VOICE_MIN_WORDS } from "../voice-intake-core";
import { initialConversationState } from "./conversation-machine";
import type {
  ConversationPhase,
  ConversationState,
} from "./conversation-types";
import { VoiceAct } from "./voice-act";

// The act that learns how this company writes: it collects a corpus, says how
// much it has, and builds a voice profile from it.
//
// The word meter is the SERVER's count, not the browser's — `initialSummary` is
// the restore probe's answer for a resumed session — so the stories that matter
// are the ones where that count is below, at, and past the threshold the build
// needs. A story that seeded the count client-side would be documenting a
// number the wire never sent.
//
// The eyebrow is the rail position, ungrouped in every locale.

function state(
  phase: ConversationPhase,
  overrides: Partial<ConversationState> = {},
): ConversationState {
  return {
    ...initialConversationState,
    act: "voice",
    phase,
    ...overrides,
  };
}

// The corpus summary is the server's whole verdict on what it holds, not a word
// count with decoration: `maturity` and `quality_band` are its own readings, and
// a fixture that invented them independently of `total_words` would document a
// summary the server cannot produce. So they are derived from the count here,
// the same way the server derives them from the corpus.
type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];

function summary(totalWords: number): CorpusSummary {
  return {
    total_words: totalWords,
    target_words: 30_000,
    maturity: totalWords < VOICE_MIN_WORDS ? "collecting" : "provisional",
    quality_band: totalWords < VOICE_MIN_WORDS ? "thin" : "good",
    source_count: Math.max(1, Math.round(totalWords / 240)),
    register_words: { email: totalWords },
  };
}

function act(
  conversation: ConversationState,
  corpus: CorpusSummary | null,
  locale?: "de",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ voice_profile: ["read", "create", "update"] }),
      "GET /voice/corpus": () =>
        corpus ? jsonResponse(corpus) : jsonResponse(summary(0)),
    });
    return (
      <StoryProviders locale={locale}>
        <VoiceAct
          state={conversation}
          dispatch={() => {}}
          initialSummary={corpus}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof VoiceAct> = {
  title: "Onboarding/Conversation/Voice act",
  component: VoiceAct,
};
export default meta;
type Story = StoryObj<typeof VoiceAct>;

/** Nothing collected yet: the act says what it wants and how to hand it over. */
export const Empty: Story = { render: act(state("vo.collecting"), null) };

/**
 * Some corpus, not enough to build from. The meter has to say how far off it is
 * rather than only refusing — a build button that is inert with no figure beside
 * it leaves the reader guessing how much more to paste.
 */
export const BelowThreshold: Story = {
  render: act(state("vo.collecting"), summary(180)),
};

/**
 * Enough to build. A four-digit word count is where de-DE grouping first shows,
 * which makes this the story where the meter's notation is visible at all.
 */
export const ReadyToBuild: Story = {
  render: act(state("vo.collecting"), summary(4820)),
};

/** The same meter in German. */
export const ReadyToBuildGerman: Story = {
  render: act(state("vo.collecting"), summary(4820), "de"),
};

/** Naming who is speaking, which the profile needs before it can build. */
export const NamingTheSpeaker: Story = {
  render: act(state("vo.speaker"), summary(4820)),
};

/** The build in flight. The thread carries the progress in words, so the orb's
 *  digits stay out of the accessibility tree rather than being announced on
 *  every tick. */
export const Building: Story = {
  render: act(
    state("vo.building", { activeBuildId: "build-1" }),
    summary(4820),
  ),
};

/** A profile built. The version it produced is a NAME — never grouped, because
 *  "1.204" is a different version from "1204" to anybody typing it in. */
export const Built: Story = {
  render: act(
    state("vo.result", {
      activeBuildId: null,
      lastBuildStatus: "succeeded",
      lastBuildStage: "activate",
    }),
    summary(4820),
  ),
};

/**
 * Skipped. A company that will not hand over its writing still has to reach the
 * end of onboarding, so this is an ordinary outcome rather than a failure — and
 * the act says what the product will do without it.
 */
export const Skipped: Story = {
  render: act(state("vo.skipped"), summary(0)),
};

/** At 390px the meter, the paste area and the build verb stack. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: act(state("vo.collecting"), summary(4820)),
};
