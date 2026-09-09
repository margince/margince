// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IntroDecisionDrawer } from "./introdecision";
import type { IntroRequest } from "./introrequests";
import { installFetchStub, StoryProviders } from "./story-utils";

// The colleague's side of an introduction ask.
//
// The one thing on this surface that is not a fact about the relationship is
// the provenance of the forwardable note, and it is the fact that changes what
// the colleague is deciding: they are about to send those words under their own
// name. So the mark is indigo when a model drafted them and absent when a
// person wrote them — the two frames below are the whole difference.
//
// EVERY INSTANT IS FIXED. `make fe-clock-drift` runs the suite at +200 days and
// requires the same verdict.

const NOTE =
  "Dana runs fleet operations at Brandt and has been asking about retrofit timelines. Worth ten minutes?";

const request: IntroRequest = {
  id: "3f7c1a90-0000-4000-8000-00000000a001",
  person_id: "3f7c1a90-0000-4000-8000-00000000c001",
  requester_user_id: "3f7c1a90-0000-4000-8000-00000000u001",
  requester_display_name: "Jonas Weber",
  introducer_user_id: "3f7c1a90-0000-4000-8000-00000000u002",
  route_type: "direct",
  internal_reason:
    "Jonas is working the depot retrofit and you are the only one Dana answers.",
  value_for_target:
    "A rollout plan for the four depots, with the downtime costed.",
  forwardable_note: NOTE,
  note_generated_by: "model",
  note_ai_generated: true,
  name_drop_allowed: true,
  fallback_policy: "name_drop",
  status: "requested",
  requested_at: "2026-08-13T09:00:00Z",
  due_at: "2026-08-20T09:00:00Z",
  version: 1,
};

function drawer(over: Partial<IntroRequest>) {
  return () => {
    // The surface only writes, and only when the colleague answers. The stub
    // is still installed so a stray read cannot leave the iframe and resolve
    // to something that looks like an answer.
    installFetchStub({});
    return (
      <StoryProviders>
        <IntroDecisionDrawer
          personId={request.person_id}
          personName="Dana Buyer"
          request={{ ...request, ...over }}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof IntroDecisionDrawer> = {
  title: "Records/Intro decision",
  component: IntroDecisionDrawer,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof IntroDecisionDrawer>;

// A note Margince drafted. The mark above the quotation is indigo, because
// indigo is what says "a machine wrote this" everywhere in the product and this
// is the sentence the colleague would be signing.
export const ModelDraftedNote: Story = { render: drawer({}) };

// The same ask with a note the requester typed. No mark at all: there is
// nothing to disclose, and a tint here would claim a writer nobody used.
export const HumanWrittenNote: Story = {
  render: drawer({ note_generated_by: "human", note_ai_generated: false }),
};

// Dark, on the drafted note: `--aiText` on the quiet badge's own ground is the
// pair the dark accent lift moves first.
export const ModelDraftedNoteDark: Story = {
  globals: { theme: "dark" },
  render: drawer({}),
};
