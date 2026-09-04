// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { initialConversationState } from "./conversation-machine";
import type { ConversationState } from "./conversation-types";
import { InviteAct } from "./invite-act";

// The question between the company and the personal steps. One state worth
// reviewing, in two languages: the surface is the same whichever card is
// picked, and Continue leaves it.

const asking: ConversationState = {
  ...initialConversationState,
  act: "invite",
  phase: "in.ask",
};

function act(locale?: "de") {
  return () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders locale={locale}>
        <InviteAct state={asking} dispatch={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof InviteAct> = {
  title: "Onboarding/Conversation/Invite act",
  component: InviteAct,
};
export default meta;
type Story = StoryObj<typeof InviteAct>;

/** The company is confirmed; will this person work in Margince too? */
export const Asking: Story = { render: act() };

/** The German act: two answers that must read as equals, not as a step and
 *  its skip. */
export const AskingGerman: Story = { render: act("de") };
