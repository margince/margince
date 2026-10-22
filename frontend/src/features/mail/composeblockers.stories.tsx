// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";
import { ComposeBlockers } from "./composeblockers";

const meta: Meta = {
  title: "Patterns/Compose mail/What stands in the way",
  component: ComposeBlockers,
};
export default meta;

const UNPROVEN = {
  allowed: false,
  recipients: [
    {
      address: "anna@example.test",
      verdict: "review",
      reason_code: "no_compatible_evidence",
      decided_by: "machine",
      would_refuse: true,
      can_be_overruled: true,
      resolved_category: "requested_followup",
    },
  ],
};

function Composer() {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /activities/act-1": () =>
      jsonResponse({
        id: "act-1",
        kind: "email",
        direction: "inbound",
        subject: "The quote",
        occurred_at: "2026-09-01T09:00:00Z",
      }),
    "GET /contacts/c-1/360": () =>
      jsonResponse({
        as_of: "2026-09-01T09:00:00Z",
        sections_omitted: [],
        contact: {
          id: "c-1",
          full_name: "Anna Brandt",
          emails: [{ email: "anna@example.test" }],
        },
      }),
    "POST /activities/act-1/send-email:preview": () => jsonResponse(UNPROVEN),
  });
  return (
    <StoryProviders>
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={() => {}}
      />
    </StoryProviders>
  );
}

/** Nothing on record says the rep may write to this address. The strip above
 *  the foot carries a reason pick that records it under their own name. */
export const ReasonMissing: StoryObj = {
  render: () => <Composer />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("To"), "anna@example.test");
    await userEvent.tab();
    await dialog.findByRole("combobox", {
      name: "Why you may write to anna@example.test",
    });
  },
};

export const ReasonMissingDark: StoryObj = {
  ...ReasonMissing,
  globals: { theme: "dark" },
};
