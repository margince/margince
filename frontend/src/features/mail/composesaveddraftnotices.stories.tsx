// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";

// What the composer says about its saved draft: put back whole or short of files, or a
// refused save with the way to retry. The real composer over stubbed routes.

const SAVED: components["schemas"]["MailDraft"] = {
  id: "md-1",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: ["buyer@acme.test"],
  cc: [],
  bcc: [],
  subject: "Pricing for 40 seats",
  body: "Following up on the seat count you asked about.",
  html_body: "<p>Following up on the seat count you asked about.</p>",
  attachments: [
    {
      id: "f-1",
      filename: "seat-pricing.pdf",
      content_type: "application/pdf",
      byte_size: 48_213,
    },
  ],
  communication_context: "requested_followup",
  version: 3,
  agent_drafted: false,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

// The autosave waits a second after the last key before it writes.
const SAVE_WAIT = { timeout: 4000 };

function refused() {
  return new Response(
    JSON.stringify({ code: "internal", title: "The draft store is down." }),
    { status: 503, headers: { "Content-Type": "application/problem+json" } },
  );
}

function composerStory(routes: RouteMap) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /mail-drafts": () => jsonResponse(SAVED),
      ...routes,
    });
    return (
      <StoryProviders>
        <ToastProvider>
          <ComposeModal
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
            open
            onClose={() => {}}
          />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

async function restored() {
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText("Saved draft restored");
  return dialog;
}

async function refusedOnScreen() {
  const dialog = await restored();
  await userEvent.type(
    dialog.getByRole("textbox", { name: "Body" }),
    " Pricing attached.",
  );
  const line = await dialog.findByRole("alert", {}, SAVE_WAIT);
  await expect(line).toHaveTextContent("The draft was not saved.");
  await expect(
    within(line).getByRole("button", { name: "Retry" }),
  ).toBeInTheDocument();
}

const meta: Meta = {
  title: "Patterns/Compose mail/Saved draft notices",
};
export default meta;

type Story = StoryObj;

/** Put back whole: the words, the chosen file and the reason for contact. */
export const RestoredWhole: Story = {
  render: composerStory({}),
  play: async () => {
    const dialog = await restored();
    await expect(dialog.getByText("seat-pricing.pdf")).toBeInTheDocument();
    await expect(
      dialog.getByRole("combobox", { name: "Reason for contact" }),
    ).toHaveTextContent("Follow-up they requested");
  },
};

/** Two chosen files can no longer be attached. The notice counts them and
 *  names none, since one the reader cannot see is not theirs to learn of. */
export const FilesUnavailable: Story = {
  render: composerStory({
    "GET /mail-drafts": () =>
      jsonResponse({ ...SAVED, attachments_unavailable: 2 }),
  }),
  play: async () => {
    const dialog = await restored();
    await expect(
      dialog.getByText("2 files could not be restored."),
    ).toBeInTheDocument();
  },
};

export const FilesUnavailablePhone: Story = {
  ...FilesUnavailable,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The save was refused. The line under the words says so and offers Retry;
 *  the next edit, or the connection coming back, tries again by itself. */
export const SaveRefused: Story = {
  render: composerStory({ "PUT /mail-drafts": refused }),
  play: refusedOnScreen,
};

export const SaveRefusedPhone: Story = {
  ...SaveRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const SaveRefusedDark: Story = {
  ...SaveRefused,
  globals: { theme: "dark" },
};
