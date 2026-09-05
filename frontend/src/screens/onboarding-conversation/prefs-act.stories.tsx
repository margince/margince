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
import { PrefsAct } from "./prefs-act";

// The last word before the app: what the agent may change on its own. The
// creator's rail and the member's differ only in the stops behind this one.

const asking: ConversationState = {
  ...initialConversationState,
  act: "prefs",
  phase: "pf.ask",
};

const autonomy = {
  data: [
    {
      kind: "close_date_correction",
      mode: "manual",
      approved_clean: 12,
      approved_edited: 1,
      rejected: 0,
    },
    {
      kind: "org_name_promotion",
      mode: "auto",
      approved_clean: 0,
      approved_edited: 0,
      rejected: 0,
    },
  ],
};

function act(admin: boolean, locale?: "de") {
  return () => {
    installFetchStub({
      "GET /me": meRoute(admin ? { installation_settings: ["update"] } : {}),
      "GET /autonomy": () => jsonResponse(autonomy),
    });
    return (
      <StoryProviders locale={locale}>
        <PrefsAct
          state={{ ...asking, memberPath: !admin }}
          dispatch={() => {}}
          persist={async () => true}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PrefsAct> = {
  title: "Onboarding/Conversation/Preferences act",
  component: PrefsAct,
};
export default meta;
type Story = StoryObj<typeof PrefsAct>;

/** The creator, on the last of six stops. */
export const Admin: Story = { render: act(true) };

/** A member, on the last of three. */
export const Member: Story = { render: act(false) };

/** The German act. */
export const AdminGerman: Story = { render: act(true, "de") };
