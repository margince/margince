// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { initialConversationState } from "./conversation-machine";
import type { ConversationState } from "./conversation-types";
import { TeamAct } from "./team-act";

// The team act: the settings invite form inside the setup journey, for a
// creator who will not work in Margince and names the first person who will.

const asking: ConversationState = {
  ...initialConversationState,
  act: "team",
  phase: "tm.ask",
};

function act(locale?: "de") {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /teams": () => jsonResponse({ data: [], next_cursor: null }),
      "POST /users/access-preview": () =>
        jsonResponse({ role: "rep", row_scope: "own", objects: {} }),
    });
    return (
      <StoryProviders locale={locale}>
        <TeamAct
          state={asking}
          dispatch={() => {}}
          persist={async () => true}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TeamAct> = {
  title: "Onboarding/Conversation/Team act",
  component: TeamAct,
};
export default meta;
type Story = StoryObj<typeof TeamAct>;

/** Nobody invited yet: the form, and a skip. */
export const Asking: Story = { render: act() };

/** The German act. */
export const AskingGerman: Story = { render: act("de") };
