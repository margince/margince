// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { VOICE_MIN_WORDS } from "../voice-intake-core";
import type { CorpusManifestEntry } from "./use-voice-corpus";
import { VoiceActArtifact } from "./voice-artifact";
import "../onboarding.css";
import "./conversation.css";

// The voice act's right panel: what the corpus holds, how far it is from the
// target, and where a running build has got to.
//
// Every number here is a SERVER number — the summary the last ingest returned
// and the per-source kept-of-total counts — so the fixtures derive the
// summary's verdicts from its word count the way the server does. A fixture
// that invented `quality_band` beside an unrelated `total_words` would
// document a summary the wire cannot send.
//
// The act's own stories (Onboarding/Conversation/Voice act) drive this through
// the reducer; these mount the panel directly, so the meter and the manifest
// can be seen at a count the act would take a paste to reach.

type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];

function summary(totalWords: number): CorpusSummary {
  return {
    total_words: totalWords,
    target_words: 30_000,
    maturity: totalWords < VOICE_MIN_WORDS ? "collecting" : "provisional",
    quality_band: totalWords < VOICE_MIN_WORDS ? "thin" : "good",
    source_count: Math.max(1, Math.round(totalWords / 240)),
    // Several registers, because the mix line under the bar is a join and a
    // single-register corpus never draws its separator.
    register_words: {
      email: Math.round(totalWords * 0.6),
      proposal: Math.round(totalWords * 0.3),
      note: Math.round(totalWords * 0.1),
    },
  };
}

// Two sources of different kinds: kept-of-total is shown only where filtering
// actually discarded turns, so a manifest with no transcript in it never draws
// the second sentence at all.
const MANIFEST: readonly CorpusManifestEntry[] = [
  {
    ref: "paste-1",
    label: "Follow-up to Brandt Automotive",
    keptWords: 1840,
    inputWords: 1840,
    transcript: false,
    lines: ["Thanks for the walk-through of the Sindelfingen line."],
  },
  {
    ref: "paste-2",
    label: "Depot survey call (transcript)",
    keptWords: 2980,
    inputWords: 7420,
    transcript: true,
    lines: [],
  },
];

const meta: Meta<typeof VoiceActArtifact> = {
  title: "Onboarding/Conversation/Voice artifact",
  component: VoiceActArtifact,
};
export default meta;
type Story = StoryObj<typeof VoiceActArtifact>;

function panel(
  props: Partial<Parameters<typeof VoiceActArtifact>[0]> = {},
  locale?: "de",
) {
  return () => (
    <StoryProviders locale={locale}>
      <VoiceActArtifact
        summary={summary(4820)}
        manifest={MANIFEST}
        stage={null}
        building={false}
        {...props}
      />
    </StoryProviders>
  );
}

/** Nothing handed over yet. The heading's body line is all there is, and it
 *  has to carry what the panel is for on its own. */
export const NothingCollected: Story = {
  render: panel({ summary: null, manifest: [] }),
};

/** A corpus below the threshold a build needs: the meter says how far off it
 *  is rather than only refusing, and the register mix under the bar says what
 *  kind of writing it holds. */
export const BelowThreshold: Story = {
  render: panel({ summary: summary(180), manifest: MANIFEST.slice(0, 1) }),
};

/** Enough to build from. A four-digit count is where de-DE grouping first
 *  shows, which makes this the story the meter's notation is visible in. */
export const ReadyToBuild: Story = { render: panel() };

/** The build in flight, at the stage the server last reported: the two before
 *  it carry a check, this one the spinner, the last one nothing yet. */
export const Building: Story = {
  render: panel({ building: true, stage: "evaluate" }),
};

/** A build that failed. The pinned bar is the act's own primary action, and
 *  only this outcome offers a retry beside Continue. */
export const BuildFailed: Story = {
  render: panel({
    continueBar: {
      reason: "failed",
      onContinue: () => {},
      onRetry: () => {},
    },
  }),
};

/** The German panel, where the meter's two halves and the mix line all run
 *  longer and must stay on their own rows. */
export const German: Story = { render: panel({}, "de") };

/** The same panel in the dark theme: the body line and the mix line are the
 *  two muted roles here, and muted is where the dark lift shows. */
export const ReadyToBuildDark: Story = {
  globals: { theme: "dark" },
  render: panel(),
};
