// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../testing/story-utils";
import { OwnerIdentitiesCard } from "./capture-owner-identities";

// A seat's own other addresses. Unlike the exclusions card beside it there is no
// scope to tell apart — every row here is the reader's own, and a colleague's
// identities are never listed — so the states worth drawing are what the list
// looks like with entries and what it says when there are none.
const ALIAS = {
  id: "oi-1",
  kind: "address",
  value: "l.jankowfsky@privat.example",
  source: "user",
  created_at: "2026-09-01T09:00:00Z",
};
const OWN_DOMAIN = {
  id: "oi-2",
  kind: "domain",
  value: "privat.example",
  source: "user",
  created_at: "2026-09-01T09:05:00Z",
};
const LEARNED = {
  id: "oi-3",
  kind: "address",
  value: "lars.forwarding-alias@old-company.example",
  source: "delivered_to",
  created_at: "2026-09-14T06:40:00Z",
};

function story(identities: Record<string, unknown>[], routes: RouteMap = {}) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/owner-identities": () => jsonResponse({ data: identities }),
      ...routes,
    });
    return (
      <StoryProviders>
        <ToastProvider>
          <OwnerIdentitiesCard />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof OwnerIdentitiesCard> = {
  title: "Settings/You/Connections/Your other addresses",
  component: OwnerIdentitiesCard,
};
export default meta;
type Story = StoryObj<typeof OwnerIdentitiesCard>;

// Both kinds and a learned alias: the caption says which kind each binds, when
// it was declared, and that the product found the third.
export const Populated: Story = {
  render: story([ALIAS, OWN_DOMAIN, LEARNED]),
};

export const PopulatedDark: Story = {
  globals: { theme: "dark" },
  render: story([ALIAS, OWN_DOMAIN, LEARNED]),
};

export const PopulatedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story([ALIAS, OWN_DOMAIN, LEARNED]),
};

// Nothing declared, which is the ordinary state and has to read as a fact rather
// than as a list that failed to draw — "I have told it about no other addresses"
// is what a reader opens this card to confirm.
export const Empty: Story = { render: story([]) };

export const Declaring: Story = {
  render: story([ALIAS]),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Add address" }),
    );
    await body.findByRole("dialog");
  },
};

const WITHDRAWN: RouteMap = {
  "DELETE /capture/owner-identities/oi-3": () =>
    new Response(null, { status: 204 }),
};

async function removeLearned(canvasElement: HTMLElement) {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(
    await body.findByRole("button", { name: `Remove ${LEARNED.value}` }),
  );
  return body;
}

// A learned address goes at once like a typed one, with Undo on the notice.
export const Removed: Story = {
  render: story([ALIAS, OWN_DOMAIN, LEARNED], WITHDRAWN),
  play: async ({ canvasElement }) => {
    const body = await removeLearned(canvasElement);
    const undo = await body.findByRole("button", { name: "Undo" });
    await waitFor(() => expect(undo).toBeVisible());
    await expect(body.queryByRole("dialog")).not.toBeInTheDocument();
  },
};

// Past its ten minutes the Undo is refused, and the notice says what to do.
export const UndoRefused: Story = {
  render: story([ALIAS, OWN_DOMAIN, LEARNED], {
    ...WITHDRAWN,
    "POST /capture/owner-identities/oi-3/restore": () =>
      jsonResponse({ code: "not_found", status: 404 }, 404),
  }),
  play: async ({ canvasElement }) => {
    const body = await removeLearned(canvasElement);
    await userEvent.click(await body.findByRole("button", { name: "Undo" }));
    await body.findByText(
      "This address can no longer be put back. Add it again instead.",
    );
  },
};
