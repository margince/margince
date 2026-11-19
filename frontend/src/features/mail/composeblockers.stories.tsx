// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { browserZone } from "../../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";
import { ComposeBlockers } from "./composeblockers";
import { editSeedOf } from "./schedulededit";

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
      evidence_subject: { entity_type: "contact", entity_id: "c-1" },
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

/** Several recipients refused for one cause: each address gets its own line
 *  with the reason. */
export const NewMessageSharedAddress: StoryObj = {
  render: () => <NewMessage preview={SHARED_ADDRESS} />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const to = dialog.getByLabelText("To");
    await userEvent.type(
      to,
      "x@one.example; y@two.example; z@three.example{Enter}",
    );
    for (const address of [
      "x@one.example",
      "y@two.example",
      "z@three.example",
    ]) {
      await dialog.findByText(new RegExp(`^${address}: `));
    }
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

// A scheduled message at the attachment cap, with a reason the record does not
// prove. The notices band and the foot are both as full as they get.
const CROWDED = editSeedOf({
  id: "019f7e65-fbf7-7114-b114-40af4af63ae9",
  status: "scheduled",
  scheduled_at: new Date(Date.now() + 86_400_000).toISOString(),
  scheduled_tz: browserZone(),
  subject: "The renewal quote",
  to: ["anna@example.test"],
  body: "Hi Anna, here is the renewal quote and everything behind it.",
  communication_context: "account_notice",
  attachments: Array.from({ length: 10 }, (_, index) => ({
    id: `att-${index + 1}`,
    filename: `Renewal_appendix_${index + 1}.pdf`,
    content_type: "application/pdf",
    byte_size: 48_000,
  })),
  links: [{ entity_type: "contact", entity_id: "c-1" }],
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
});

const UNPROVEN_WITH_ALTERNATIVE = {
  allowed: false,
  recipients: [
    {
      ...UNPROVEN.recipients[0],
      supported_alternatives: ["requested_followup"],
    },
  ],
};

function boxOf(selector: string): DOMRect {
  const node = document.querySelector(selector);
  if (!node) {
    throw new Error(`${selector} is not drawn`);
  }
  return node.getBoundingClientRect();
}

/** Notices, ten files and a scheduled moment at once: the message keeps a
 *  readable height, and nothing above the foot covers it. */
export const CrowdedKeepsTheMessage: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /contacts/c-1/360": ANNA,
      "POST /emails:preview": () => jsonResponse(UNPROVEN_WITH_ALTERNATIVE),
    });
    return (
      <StoryProviders>
        <ComposeModal
          edit={CROWDED}
          entityType="contact"
          entityId="c-1"
          contactId="c-1"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("No recorded reason to contact this recipient");
    await dialog.findByText("Renewal_appendix_10.pdf");
    const rem = Number.parseFloat(
      getComputedStyle(document.documentElement).fontSize,
    );
    await expect(boxOf(".compose-sheet").height).toBeGreaterThanOrEqual(
      8 * rem,
    );
    await expect(boxOf(".compose-blockers").bottom).toBeLessThanOrEqual(
      boxOf(".compose-foot").top,
    );
  },
};

export const CrowdedKeepsTheMessageDark: StoryObj = {
  ...CrowdedKeepsTheMessage,
  globals: { theme: "dark" },
};

export const CrowdedKeepsTheMessagePhone: StoryObj = {
  ...CrowdedKeepsTheMessage,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
