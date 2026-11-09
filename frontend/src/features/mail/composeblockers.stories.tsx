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

const NO_PURPOSE = {
  allowed: false,
  recipients: [
    {
      address: "anna@example.test",
      verdict: "deny",
      reason_code: "unknown_purpose",
      decided_by: "machine",
      would_refuse: true,
      can_be_overruled: false,
    },
  ],
};

const OBJECTED = {
  allowed: false,
  recipients: [
    {
      address: "anna@example.test",
      verdict: "deny",
      reason_code: "marketing_objection",
      decided_by: "subject",
      would_refuse: true,
      can_be_overruled: false,
    },
  ],
};

const SHARED_ADDRESS = {
  allowed: false,
  recipients: ["x@one.example", "y@two.example", "z@three.example"].map(
    (address) => ({
      address,
      verdict: "deny",
      reason_code: "recipient_resolves_to_no_single_subject",
      decided_by: "machine",
      would_refuse: true,
      can_be_overruled: false,
    }),
  ),
};

const ANNA = () =>
  jsonResponse({
    as_of: "2026-09-01T09:00:00Z",
    sections_omitted: [],
    contact: {
      id: "c-1",
      full_name: "Anna Brandt",
      emails: [{ email: "anna@example.test" }],
    },
  });

function NewMessage({ preview }: Readonly<{ preview: object }>) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /contacts/c-1/360": ANNA,
    "POST /emails:preview": () => jsonResponse(preview),
  });
  return (
    <StoryProviders>
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={() => {}}
      />
    </StoryProviders>
  );
}

function Composer() {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read", "update"],
      activity: ["read", "update"],
    }),
    "GET /activities/act-1": () =>
      jsonResponse({
        id: "act-1",
        kind: "email",
        direction: "inbound",
        subject: "The quote",
        occurred_at: "2026-09-01T09:00:00Z",
      }),
    "GET /contacts/c-1/360": ANNA,
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

export const ReasonMissing: StoryObj = {
  render: () => <Composer />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("To"), "anna@example.test");
    await userEvent.tab();
    await dialog.findByRole("button", { name: "Record what happened" });
  },
};

export const ReasonMissingDark: StoryObj = {
  ...ReasonMissing,
  globals: { theme: "dark" },
};

async function addressAnna() {
  const dialog = within(await screen.findByRole("dialog"));
  await userEvent.type(dialog.getByLabelText("To"), "anna@example.test");
  await userEvent.tab();
  return dialog;
}

/** A new message with no reason picked: the strip alone says so, because the
 *  engine's missing-purpose refusal asks the same question. */
export const NewMessageNoReason: StoryObj = {
  render: () => <NewMessage preview={NO_PURPOSE} />,
  play: async () => {
    const dialog = await addressAnna();
    await dialog.findByText("No contact reason.");
  },
};

export const NewMessageNoReasonDark: StoryObj = {
  ...NewMessageNoReason,
  globals: { theme: "dark" },
};

/** A refusal for another cause stands beside the strip. */
export const NewMessageObjected: StoryObj = {
  render: () => <NewMessage preview={OBJECTED} />,
  play: async () => {
    const dialog = await addressAnna();
    await dialog.findByText("Message cannot be sent");
  },
};

/** Several recipients refused for one cause: the line saying whom it applies
 *  to stands on its own line under the explanation. */
export const NewMessageSharedAddress: StoryObj = {
  render: () => <NewMessage preview={SHARED_ADDRESS} />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const to = dialog.getByLabelText("To");
    await userEvent.type(
      to,
      "x@one.example; y@two.example; z@three.example{Enter}",
    );
    await dialog.findByText(/This applies to x@one.example and 2 other/);
  },
};

export const NewMessageSharedAddressDark: StoryObj = {
  ...NewMessageSharedAddress,
  globals: { theme: "dark" },
};

export const NewMessageSharedAddressPhone: StoryObj = {
  ...NewMessageSharedAddress,
  tags: ["uat-phone"],
};

export const NewMessageObjectedDark: StoryObj = {
  ...NewMessageObjected,
  globals: { theme: "dark" },
};

export const NewMessageNoReasonPhone: StoryObj = {
  ...NewMessageNoReason,
  tags: ["uat-phone"],
};
