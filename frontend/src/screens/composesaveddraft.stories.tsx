// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { ComposeModal } from "./compose";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The rep's own unsent message as the composer holds it: put back when the
// composer opens, saved beside Send, and refused when another window moved it.
// Every frame is the real composer over stubbed routes, driven the way a reader
// would drive it.

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
  version: 3,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

function versionSkew() {
  return new Response(
    JSON.stringify({ code: "version_skew", title: "Conflict" }),
    { status: 409, headers: { "Content-Type": "application/problem+json" } },
  );
}

function savedDraftStory(routes: RouteMap) {
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

// The composer is a portalled Modal, so every query reaches the document.
async function restoredOnScreen() {
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText("Saved draft restored");
  await expect(
    dialog.getByDisplayValue("Pricing for 40 seats"),
  ).toBeInTheDocument();
}

const meta: Meta = {
  title: "Patterns/Compose mail/Saved draft",
};
export default meta;

type Story = StoryObj;

/** Opened over a message the rep left unsent: the words are back, and the
 *  notice above them names that and offers to delete it. */
export const Restored: Story = {
  render: savedDraftStory({}),
  play: restoredOnScreen,
};

/** Save as draft, beside Send: the draft is written and a toast confirms it,
 *  carrying the one verb that takes it back. */
export const SavedAsDraft: Story = {
  render: savedDraftStory({
    "PUT /mail-drafts": () => jsonResponse({ ...SAVED, version: 4 }),
  }),
  play: async () => {
    await restoredOnScreen();
    await userEvent.type(
      screen.getByRole("textbox", { name: "Body" }),
      " Pricing attached.",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Save as draft" }),
    );
    await screen.findByText("Draft saved");
  },
};

/** Another window saved over this draft first. Nothing is overwritten: the
 *  text on screen stays, and the reader chooses which version wins. */
export const ChangedElsewhere: Story = {
  render: savedDraftStory({ "PUT /mail-drafts": versionSkew }),
  play: async () => {
    await restoredOnScreen();
    await userEvent.type(
      screen.getByRole("textbox", { name: "Body" }),
      " Pricing attached.",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Save as draft" }),
    );
    await screen.findByText("Draft changed in another window");
  },
};

/** The restored notice in dark, where its ink re-resolves. */
export const RestoredDark: Story = {
  globals: { theme: "dark" },
  render: savedDraftStory({}),
  play: restoredOnScreen,
};
