// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealPulse } from "./dealpulse";

// The deal record's opening sentence. The four readings that used to sit here
// moved into the cockpit band (dealcockpit.stories.tsx), the facts beside the
// name into dealheaderfacts.stories.tsx, and the seats into the committee card,
// which draws them with the verbs that change them (dealcommittee.stories.tsx).
//
// The state worth seeing is the one easy to get wrong, and it is the story
// below: a card still loading renders identically to a healthy one if its
// distinction is dropped, which is why it is here rather than a happy path
// alone.

const meta: Meta = {
  title: "Records/Deal 360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL_ID = "01a03000-0000-7000-8000-0000000000aa";

/** A deal that needs somebody: our move, waiting since the pilot review. */
export const NeedsYou: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse
        card={
          {
            deal_id: DEAL_ID,
            story: { sentences: [] },
            reply_to: MAIL_ID,
            next: {
              action: "draft_email",
              reason: "Answer them.",
              evidence: [
                {
                  text: "Unanswered: Slots for the pilot review",
                  activity_id: MAIL_ID,
                  occurred_at: "2026-05-20T09:00:00Z",
                },
              ],
            },
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "model",
          } as components["schemas"]["DealStatusCard"]
        }
        timeline={[]}
      />
    </StoryProviders>
  ),
};

/** Nobody here is owed an answer. */
export const TheirMove: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse
        card={
          {
            deal_id: DEAL_ID,
            story: { sentences: [] },
            reply_to: null,
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "model",
          } as components["schemas"]["DealStatusCard"]
        }
        timeline={[]}
      />
    </StoryProviders>
  ),
};

/** Nothing read yet: the sentence draws nothing rather than guessing. */
export const Loading: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse card={undefined} timeline={[]} />
    </StoryProviders>
  ),
};
