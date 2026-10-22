// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { DraftBand, RewriteRow } from "./composedraftband";
// The drawer's own sheet: `.compose-band-body` and `.compose-rewrite` are
// reached BY CLASS, and a story's module graph stops short of compose.tsx,
// which loads it.
import "./compose.css";

// Provenance stays visible above the controls that request another draft.

type EmailDraft = components["schemas"]["EmailDraft"];
type DraftReason = components["schemas"]["AccountDraftReason"];
type Maturity = components["schemas"]["VoiceProfile"]["maturity"];

type Provenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version" | "voice_degraded"
>;

const WROTE_IT: Provenance = {
  ai_generated: true,
  ai_disclosure: "Drafted with AI assistance. Review before sending.",
  voice_profile_version: 1234,
  voice_degraded: false,
};

// Both shapes a reason comes in: one carrying a record the reader can open, and
// the rep's own steer, which has nothing behind it to open.
const REASONS: readonly DraftReason[] = [
  { kind: "intent", label: "Ask for the signed order" },
  {
    kind: "deal",
    label: "Renewal closes this quarter",
    evidence_ref: {
      entity_type: "deal",
      entity_id: "deal-1",
      name: "Acme Renewal",
    },
  },
];

function band(provenance: Provenance, maturity: Maturity | undefined) {
  return () => (
    <StoryProviders>
      <DraftBand
        provenance={provenance}
        maturity={maturity}
        reasons={[...REASONS]}
        actions={<RewriteRow onRewrite={() => {}} disabled={false} />}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof DraftBand> = {
  title: "Patterns/Compose mail/Draft band",
  component: DraftBand,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DraftBand>;

/** A draft nobody typed: the disclosure, the rewrites a rep may ask of it, what
 *  it was written from, and the profile version that styled it. */
export const MachineWroteIt: Story = { render: band(WROTE_IT, "building") };

/** The profile is still provisional. The label reports what the voice is TODAY
 *  and says nothing about this draft — nothing gates drafting on maturity. */
export const ProvisionalVoice: Story = {
  render: band(WROTE_IT, "provisional"),
};

/** The sender's voice could not even be looked up. The text will not show the
 *  loss, so the band says it — the one thing a reader cannot detect by reading. */
export const VoiceCouldNotBeLookedUp: Story = {
  render: band({ ...WROTE_IT, voice_degraded: true }, "building"),
};

/** No voice profile styled this draft, so there is no served version to report
 *  a maturity over and the version line withdraws. */
export const NoVoiceProfile: Story = {
  render: band({ ...WROTE_IT, voice_profile_version: null }, undefined),
};

/** The server sent no disclosure line. The notice discloses anyway: a missing
 *  line may not silently become a missing disclosure. */
export const DisclosureLineMissing: Story = {
  render: band({ ...WROTE_IT, ai_disclosure: null }, "building"),
};

/** The same card in dark, where the indigo ground and its text token have to
 *  stay a legible pair. */
export const MachineWroteItDark: Story = {
  globals: { theme: "dark" },
  render: band(WROTE_IT, "building"),
};

// The four rewrites sit beside the disclosure over the machine's own untouched
// words. They are offered only while the body is still what the model wrote,
// which is the caller's condition. These two frames are the row's own states.

/** Ready: one press each, and each is an instruction for ONE call. */
export const RewritesReady: Story = {
  render: () => (
    <StoryProviders>
      <RewriteRow onRewrite={() => {}} disabled={false} />
    </StoryProviders>
  ),
};

/** A draft already in flight. The row refuses to start a second one — a rewrite
 *  REPLACES the body, and two in flight land whichever answers last. */
export const RewritesBarred: Story = {
  render: () => (
    <StoryProviders>
      <RewriteRow onRewrite={() => {}} disabled />
    </StoryProviders>
  ),
};

export const Dark: Story = {
  render: band(WROTE_IT, "provisional"),
  globals: { theme: "dark" },
};
