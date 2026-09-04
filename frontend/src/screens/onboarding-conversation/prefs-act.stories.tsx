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

// The last word before the app, in the two shapes it takes: an admin sees the
// installation's reporting basis above what the agent may change on its own;
// a member sees only the second, because the first is not theirs to change.

const asking: ConversationState = {
  ...initialConversationState,
  act: "prefs",
  phase: "pf.ask",
};

const settings = {
  name: "Brandt Automotive",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  base_currency_locked: false,
  max_upload_bytes: 26214400,
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
      "GET /installation/settings": () => jsonResponse(settings),
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

/** An admin: the reporting basis, prefilled, above the autonomy switches. */
export const Admin: Story = { render: act(true) };

/** A member: only what the agent may change on their own behalf. */
export const Member: Story = { render: act(false) };

/** The German act. */
export const AdminGerman: Story = { render: act(true, "de") };
