// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { CONNECTED_ROSTER } from "../../testing/mailroster.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeWindow } from "./composewindow";

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
  agent_drafted: false,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

// The send's own refusal: a follow-up on a deal alone, with no request on record.
const DEAL_NOT_REQUESTED = {
  code: "consent_not_granted",
  title: "Conflict",
  status: 409,
  details: {
    review_id: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0003",
    available_actions: [],
    refusals: [
      {
        address: "buyer@acme.test",
        reason_code: "deal_not_requested",
        category: "active_deal_followup",
      },
    ],
    review: {
      id: "01a0aaaa-bbbb-7ccc-8ddd-eeeeffff0003",
      state: "needs_context",
      reason_code: "deal_not_requested",
      refusals: [
        {
          address: "buyer@acme.test",
          reason_code: "deal_not_requested",
          category: "active_deal_followup",
        },
      ],
    },
  },
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
      "GET /connectors": () => jsonResponse(CONNECTED_ROSTER),
      "GET /mail-drafts": () => jsonResponse(SAVED),
      ...routes,
    });
    return (
      <StoryProviders>
        <ToastProvider>
          <ComposeWindow
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

/** Closed over unsaved words: the draft is written and a toast confirms it,
 *  carrying the one verb that takes it back. */
export const SavedOnClose: Story = {
  render: savedDraftStory({
    "PUT /mail-drafts": () => jsonResponse({ ...SAVED, version: 4 }),
  }),
  play: async () => {
    await restoredOnScreen();
    await userEvent.type(
      screen.getByRole("textbox", { name: "Body" }),
      " Pricing attached.",
    );
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    const toast = await screen.findByText("Draft saved", {
      selector: ".toast-said",
    });
    const region = toast.closest("output");
    if (!region) throw new Error("the saved draft drew no toast");
    await expect(
      within(region).getByRole("button", { name: "Delete" }),
    ).toBeInTheDocument();
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
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await screen.findByText("Draft changed in another window");
  },
};

/** The restored notice in dark, where its ink re-resolves. */
export const RestoredDark: Story = {
  globals: { theme: "dark" },
  render: savedDraftStory({}),
  play: restoredOnScreen,
};

export const AIDraftRestored: Story = {
  render: savedDraftStory({
    "GET /mail-drafts": () =>
      jsonResponse({
        ...SAVED,
        editor_state: {
          intent: "Ask about the seat count",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          served_body: SAVED.body,
        },
      }),
  }),
  play: async () => {
    await restoredOnScreen();
    await screen.findByRole("heading", { name: "AI-assisted draft" });
  },
};

export const AIDraftRestoredDark: Story = {
  ...AIDraftRestored,
  globals: { theme: "dark" },
};

export const RestoredDealContext: Story = {
  render: savedDraftStory({
    "GET /mail-drafts": () =>
      jsonResponse({
        ...SAVED,
        communication_context: "active_deal_followup",
        editor_state: {
          filing: [{ anchor_type: "deal", anchor_id: "deal-scope" }],
        },
      }),
    "GET /contacts/p-1/360": () =>
      jsonResponse({
        as_of: "2026-09-20T09:05:00Z",
        sections_omitted: [],
        contact: { id: "p-1", full_name: "Buyer" },
        deal_roles: {
          data: [
            {
              relationship_id: "seat-scope",
              deal_id: "deal-scope",
              deal_title: "Seat expansion",
              deal_status: "open",
              seat_current: true,
              role: "champion",
            },
          ],
          page: { has_more: false },
        },
      }),
    "POST /emails": () => jsonResponse(DEAL_NOT_REQUESTED, 409),
  }),
  play: async () => {
    await restoredOnScreen();
    const deal = await screen.findByRole("combobox", {
      name: "Deal this follow-up is about",
    });
    await expect(deal).toHaveTextContent("Seat expansion");
    // The composer asks the engine when Send is pressed, not before.
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await screen.findByText(
      /No reason to contact buyer@acme.test is on record/,
    );
  },
};

export const RestoredDealContextPhone: Story = {
  ...RestoredDealContext,
  tags: ["uat-phone"],
};
