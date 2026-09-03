// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { PreferenceCenterScreen } from "./preferences";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The public, anonymous preference center (G-6/G-7): no session, no
// workspace header — the token in the URL is the whole capability.
// PreferenceCenter is {purposes: [{key, label, state, locked}]} — no
// events — matching preferences.test.tsx's CENTER fixture exactly.
//
// A purpose's `label` here is the CATALOG's word and is only what the screen
// draws for a key `PURPOSE_LABEL_KEYS` does not name (preferences.logic.ts).
// `marketing_email` IS named there, so the row reads "Product news" and not the
// fixture's "Product updates" — which is what these stories used to query for,
// having been written before the copy moved.

const CENTER = {
  purposes: [
    {
      key: "transactional",
      label: "Deal & service messages",
      state: "granted",
      locked: true,
    },
    {
      key: "marketing_email",
      label: "Product updates",
      state: "granted",
      locked: false,
    },
    { key: "events", label: "Events", state: "unknown", locked: false },
  ],
};

function center(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <PreferenceCenterScreen token="tok-123" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PreferenceCenterScreen> = {
  title: "Signed out/Email preference centre",
  component: PreferenceCenterScreen,
};
export default meta;

type Story = StoryObj<typeof PreferenceCenterScreen>;

export const Default: Story = {
  render: center({
    "GET /public/preferences/tok-123": () => jsonResponse(CENTER),
  }),
};

// A staged (unsaved) toggle: the save bar names exactly what would be sent.
export const Dirty: Story = {
  render: center({
    "GET /public/preferences/tok-123": () => jsonResponse(CENTER),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("checkbox", { name: /product news/i }),
    );
  },
};

// G-7: the RFC 8058 one-click landing — every non-locked purpose withdrawn
// in one call, with an explicit-opt-in undo rather than a silent re-grant.
export const OneClickLanding: Story = {
  render: center({
    "GET /public/preferences/tok-123": () => jsonResponse(CENTER),
    "POST /public/preferences/tok-123/unsubscribe": () =>
      jsonResponse({ unsubscribed: ["marketing_email"] }),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: /stop everything I can switch off/i,
      }),
    );
  },
};

// PUT loops choices in separate transactions (handlers_public.go): a
// mid-list failure leaves earlier choices committed, so the card re-reads
// rather than trust the optimistic draft — the honest "may have been saved"
// banner, not a silent success.
export const PartialSave: Story = {
  render: center({
    "GET /public/preferences/tok-123": () => jsonResponse(CENTER),
    "PUT /public/preferences/tok-123": () =>
      jsonResponse(
        {
          title: "not a tracked consent purpose",
          status: 422,
          code: "invalid",
        },
        422,
      ),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("checkbox", { name: /product news/i }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: /save preferences/i }),
    );
    await canvas.findByText(/some of your choices may have been saved/i);
  },
};

// An unknown or revoked token both read as a 404 — this surface must never
// become an oracle for whether an address is known, so the copy is identical
// either way.
export const InvalidLink: Story = {
  render: center({
    "GET /public/preferences/tok-123": () =>
      jsonResponse({ title: "not found", status: 404 }, 404),
  }),
};
