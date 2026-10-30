// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { candidateVersion } from "../../testing/voice-candidate";
import { VoiceDnaCard } from "./voice-dna";

// The Settings Voice DNA card off canned profile/corpus reads — never a live
// call. Covers the owner with no profile yet, the owner still collecting (no
// derived voice text to show), a ready profile with a full corpus, a corpus row
// the build excluded from that corpus, and the read-only seat.
//
// A profile that exists is three cards, and every decision inside one is a
// settings ROW: the preferences editor is a stacked row in the voice card, the
// derived text sits behind a disclosure beside it, the corpus manifest takes
// the full width of its own card with the add verbs under it, and the build
// verb is a row whose description carries the distance still to go.

type VoiceProfile = components["schemas"]["VoiceProfile"];
type VoiceCorpusSource = components["schemas"]["VoiceCorpusSource"];
type VoiceCorpusSummary = components["schemas"]["VoiceCorpusSummary"];

const PROFILE: VoiceProfile = {
  id: "vp-1",
  owner_id: "u1",
  status: "ready",
  maturity: "building",
  quality_band: "good",
  voice_profile_md: "Short sentences. Concrete nouns. No hedging.",
  profile_version: 3,
  personality_md: "Warm but brief.",
  auto_learning_enabled: false,
  active_source_hash: "h1",
  candidate_version: null,
  last_built_at: "2026-07-01T00:00:00Z",
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  archived_at: null,
};

const SUMMARY: VoiceCorpusSummary = {
  total_words: 12400,
  target_words: 30000,
  maturity: "building",
  quality_band: "good",
  source_count: 2,
  register_words: { email: 9400, spoken: 3000 },
};

const SOURCE: VoiceCorpusSource = {
  id: "vs-1",
  origin: "manual",
  kind: "email",
  register: "email",
  weight: 1,
  source_label: "Sent mail, Q2",
  source_ref: "settings:paste:1",
  word_count: 9400,
  included: true,
  exclusion_reason: null,
  extractor_version: "1",
  occurred_at: "2026-06-01T00:00:00Z",
  retention_until: null,
  content_erased_at: null,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
  archived_at: null,
};

const SPOKEN_SOURCE: VoiceCorpusSource = {
  ...SOURCE,
  id: "vs-3",
  kind: "transcript",
  register: "spoken",
  source_label: "Discovery call, Nordwerk",
  word_count: 3000,
};

// A profile that exists but has not built a derived voice yet: status isn't
// "ready", so the card falls back to DerivedVoice's empty placeholder instead
// of quoting a voice_profile_md nobody has produced.
const COLLECTING_PROFILE: VoiceProfile = {
  ...PROFILE,
  status: "collecting",
  maturity: "collecting",
  quality_band: "thin",
  voice_profile_md: "",
  profile_version: 0,
  active_source_hash: null,
  last_built_at: null,
};

const COLLECTING_SUMMARY: VoiceCorpusSummary = {
  total_words: 420,
  target_words: 30000,
  maturity: "collecting",
  quality_band: "thin",
  source_count: 1,
  register_words: { general: 420 },
};

const COLLECTING_SOURCE: VoiceCorpusSource = {
  ...SOURCE,
  register: "general",
  word_count: 420,
};

// A source the build dropped (too short, a duplicate, …): still listed so its
// owner can see why it isn't counted, marked "excluded" rather than removed
// from the manifest outright.
const EXCLUDED_SOURCE: VoiceCorpusSource = {
  ...SOURCE,
  id: "vs-2",
  source_label: "Old boilerplate signature",
  included: false,
  exclusion_reason: "too_short",
  word_count: 40,
};

const LEARNING = {
  drafted: 6,
  accepted: 2,
  edited_sent: 3,
  rejected: 1,
  qualifying_source_count: 1,
  qualifying_words: 420,
  transformations: [],
};

// The version a finished build left active: what it learned, quoted from the
// owner's own corpus. The settings page leads with this, so a story without it
// draws a voice page that has nothing to say about the voice.
const ACTIVE_VERSION = {
  ...candidateVersion([]),
  id: "vv-3",
  profile_id: "vp-1",
  profile_version: 3,
  status: "active",
  model_name: "claude-haiku-4.5",
  profile_json: {
    inference: {
      identity_summary:
        "Plain, specific and quick to the ask. Writes like someone who has already read the contract.",
      thinking_pattern:
        "Leads with the constraint, then the offer, then one question.",
      observed_obsessions: ["delivery dates", "who signs", "next step"],
      signature_moves: [
        {
          move: "Names the blocker before the ask",
          quote: "Depot's offline the 14th, can we sign before then?",
        },
        { move: "Closes on one question", quote: "Does Thursday work?" },
        {
          move: "Puts the number in the first line",
          quote: "€48k, two phases, first one live in May.",
        },
      ],
      avoid: ["exclamation marks", "hedging", "corporate filler"],
    },
    sample_drafts: [
      {
        subject: "Retrofit timeline",
        body: "Lars, the depot window moved to the 14th. Can you confirm the quote before then? Does Thursday work for a ten-minute call?",
        voice_score: 0.86,
      },
    ],
    guidance: { next_best_key: "add_transcript" },
  },
  stats_json: { word_count: 12400, mean_sentence_words: 11.5, sample_count: 2 },
};

// The card's whole subtree reads the profile, its corpus, its version history
// and its learning aggregate; a story serves all four so no panel renders an
// error state it did not mean to capture.
function voiceStory(
  routes: RouteMap,
  seat: "full" | "read" = "full",
  versions: readonly unknown[] = [],
) {
  return () => {
    installFetchStub({
      // Every control here mutates, so the card asks useCanWrite — grant AND
      // seat. Both halves are named because they fail the same way on screen,
      // and a read-only story has to withdraw both or it is not the posture it
      // claims to draw.
      "GET /me": meRoute(
        {
          voice_profile:
            seat === "read" ? ["read"] : ["read", "create", "update"],
        },
        { seat },
      ),
      "GET /voice-profiles/vp-1/versions": () =>
        jsonResponse({ data: versions, page: emptyPage.page }),
      "GET /voice-profiles/vp-1/deltas": () => jsonResponse(emptyPage),
      "GET /voice-profiles/vp-1/learning": () => jsonResponse(LEARNING),
      ...routes,
    });
    return (
      <StoryProviders>
        <VoiceDnaCard />
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  // Under the tab's own name, exactly as SETTINGS_TABS spells it, so the three
  // voice leaves sit together instead of one of them under a root of its own.
  title: "Settings/You/Writing voice/Voice DNA",
};
export default meta;

type Story = StoryObj;

// No profile yet: the page promises a voice, walks through the three steps and
// takes the first sample, which mints the profile. An owner who skipped the
// onboarding voice step lands here.
export const Empty: Story = {
  render: voiceStory({
    "GET /voice-profiles": () => jsonResponse(emptyPage),
  }),
};

// A built profile: the derived voice, the corpus meter and its register mix,
// the preferences editor, and the build control.
export const Ready: Story = {
  render: voiceStory({
    "GET /voice-profiles": () =>
      jsonResponse({ data: [PROFILE], page: emptyPage.page }),
    "GET /voice-profiles/vp-1/sources": () =>
      jsonResponse({ data: [SOURCE], summary: SUMMARY }),
  }),
};

// A built voice: what the active build learned leads the page, ahead of the
// preferences, the corpus and the builds that feed it.
export const Built: Story = {
  render: voiceStory(
    {
      "GET /voice-profiles": () =>
        jsonResponse({ data: [PROFILE], page: emptyPage.page }),
      "GET /voice-profiles/vp-1/sources": () =>
        jsonResponse({ data: [SOURCE, SPOKEN_SOURCE], summary: SUMMARY }),
    },
    "full",
    [ACTIVE_VERSION],
  ),
};

// A profile that exists but hasn't built a voice yet: the derived-voice panel
// falls back to its empty placeholder (voice_profile_md is "" pre-build)
// instead of quoting text nobody produced, while the corpus/build controls
// underneath are already live.
export const Collecting: Story = {
  render: voiceStory({
    "GET /voice-profiles": () =>
      jsonResponse({ data: [COLLECTING_PROFILE], page: emptyPage.page }),
    "GET /voice-profiles/vp-1/sources": () =>
      jsonResponse({
        data: [COLLECTING_SOURCE],
        summary: COLLECTING_SUMMARY,
      }),
  }),
};

// Past the floor with nothing built yet: the hero ticks the first step and
// offers the first build where the writer is already looking.
export const ReadyToBuild: Story = {
  render: voiceStory({
    "GET /voice-profiles": () =>
      jsonResponse({
        data: [{ ...COLLECTING_PROFILE, maturity: "provisional" }],
        page: emptyPage.page,
      }),
    "GET /voice-profiles/vp-1/sources": () =>
      jsonResponse({
        data: [COLLECTING_SOURCE],
        summary: {
          ...COLLECTING_SUMMARY,
          total_words: 2400,
          maturity: "provisional",
          register_words: { general: 2400 },
        },
      }),
  }),
};

// Dark below the 800-word floor. The corpus meter and the floor meter stand
// together here, both drawn from tokens, beside the thin band.
export const CollectingDark: Story = {
  globals: { theme: "dark" },
  render: voiceStory({
    "GET /voice-profiles": () =>
      jsonResponse({ data: [COLLECTING_PROFILE], page: emptyPage.page }),
    "GET /voice-profiles/vp-1/sources": () =>
      jsonResponse({
        data: [COLLECTING_SOURCE],
        summary: COLLECTING_SUMMARY,
      }),
  }),
};

// A corpus row the build excluded (too short, a duplicate, …): still listed
// — never silently dropped — and marked so its owner can see why it doesn't
// count toward the meter.
export const ExcludedSource: Story = {
  render: voiceStory({
    "GET /voice-profiles": () =>
      jsonResponse({ data: [PROFILE], page: emptyPage.page }),
    "GET /voice-profiles/vp-1/sources": () =>
      jsonResponse({ data: [SOURCE, EXCLUDED_SOURCE], summary: SUMMARY }),
  }),
};

// A seat that may READ a voice but not change one. The posture is stated once at
// the top and the write affordances are then absent: the preferences box is
// readOnly with no Save under it, the corpus rows keep their facts and lose
// their remove verbs, and the build row keeps the sentence about how far the
// corpus still has to go while offering no verb to act on it.
export const ReadOnly: Story = {
  render: voiceStory(
    {
      "GET /voice-profiles": () =>
        jsonResponse({ data: [COLLECTING_PROFILE], page: emptyPage.page }),
      "GET /voice-profiles/vp-1/sources": () =>
        jsonResponse({
          data: [COLLECTING_SOURCE],
          summary: COLLECTING_SUMMARY,
        }),
    },
    "read",
  ),
};

export const ReadyAtPhoneWidth: Story = {
  ...Ready,
  tags: ["uat-phone"],
};
