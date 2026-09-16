// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../api/schema";
import { DraftBand, RewriteRow } from "./composedraftband";
import { DraftOffer } from "./composedraftcontext";
import { StoryProviders } from "./story-utils";
// The drawer's own sheet: `.compose-band` and `.compose-rewrite` are reached BY
// CLASS, and a story's module graph stops short of compose.tsx, which loads it.
import "./compose.css";

// The card that says a MACHINE wrote the words below, and the rewrites offered
// over them. What differs between the frames is what the band has to ADMIT: a
// disclosure the server omitted, a voice that could not be looked up, a profile
// still provisional. None of those makes the draft weaker, and none may be
// quietly dropped — Art. 50 is why this card is the loudest thing in the drawer.
//
// The band's child is the offer the composer puts there, so a frame shows the
// block as the drawer assembles it rather than a slot filled for the picture.

type EmailDraft = components["schemas"]["EmailDraft"];
type DraftReason = components["schemas"]["AccountDraftReason"];
type Maturity = components["schemas"]["VoiceProfile"]["maturity"];

type Provenance = Pick<
  EmailDraft,
  "ai_generated" | "ai_disclosure" | "voice_profile_version" | "voice_degraded"
>;

const WROTE_IT: Provenance = {
  ai_generated: true,
  ai_disclosure:
    "This message was drafted with AI assistance and reviewed by the sender.",
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

// The offer owns no state, so the story lends it some: the intent field is half
// of what the band's child IS, and a field nobody can type in shows the other
// half only. `run` is inert — the catalog has no model behind the button, and a
// story that faked a draft arriving would picture words no endpoint wrote.
function Offer() {
  const [intent, setIntent] = useState("Ask for the signed order");
  return (
    <DraftOffer
      intent={intent}
      onIntentChange={setIntent}
      draft={{ run: () => {}, pending: false, disabled: false, error: null }}
      unavailable={null}
    />
  );
}

function band(provenance: Provenance, maturity: Maturity | undefined) {
  return () => (
    <StoryProviders>
      <DraftBand
        provenance={provenance}
        maturity={maturity}
        reasons={[...REASONS]}
      >
        <Offer />
      </DraftBand>
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

/** A draft nobody typed: the disclosure, what it was written from, the profile
 *  version that styled it, and the steer that asks for another. */
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

/** The server sent no disclosure line. The card discloses anyway: a missing
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

// The four rewrites sit under the machine's own untouched words, in the drawer
// below the band. Offered at all only while the body is still what the model
// wrote, which is the caller's condition — these two frames are the states the
// row itself has.

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
