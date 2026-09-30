// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { SharedLinksButton } from "./analytics.sharelist";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The drawer that lists the forecast links a reader issued and that still
// open. Every state is behind the trigger, so each play opens it first; the
// drawer portals to document.body, which is why everything inside is found
// through `screen` rather than the canvas.

const TEAM_NORTH = "5f0d7a1e-8c2b-4d3a-9e61-0a4b2c9d7e11";
const TEAM_LONG = "7b2e9c40-1d5f-4a8e-b3c6-2f9e0d4a6b21";
const GONE_TEAM = "9c1a3e5f-7b2d-4f60-8a4e-3d2c1b0a9f31";

const context = () =>
  jsonResponse({
    default_scope: { kind: "workspace", label: "Whole company" },
    allowed_scopes: [
      { kind: "workspace", label: "Whole company" },
      { kind: "team", id: TEAM_NORTH, label: "Team North" },
      {
        kind: "team",
        id: TEAM_LONG,
        label:
          "Enterprise accounts, Central and Eastern Europe, including Austria and Switzerland",
      },
    ],
    capabilities: {
      view_manager_forecast: true,
      submit_manager_forecast: true,
    },
  });

function share(
  id: string,
  kind: "live" | "snapshot",
  scope: { scope_kind: "workspace" | "team" | "owner"; scope_id?: string },
  day: number,
) {
  const created = `2026-09-${String(day).padStart(2, "0")}T09:00:00Z`;
  const expires = `2026-10-${String(day).padStart(2, "0")}T09:00:00Z`;
  return {
    id,
    kind,
    target: "forecast",
    ...scope,
    created_at: created,
    expires_at: expires,
  };
}

const THREE = [
  share("share-3", "live", { scope_kind: "team", scope_id: TEAM_NORTH }, 12),
  share("share-2", "snapshot", { scope_kind: "workspace" }, 8),
  share("share-1", "live", { scope_kind: "team", scope_id: GONE_TEAM }, 3),
];

const MANY = Array.from({ length: 14 }, (_, index) =>
  share(
    `share-many-${index}`,
    index % 3 === 0 ? "snapshot" : "live",
    index % 2 === 0
      ? { scope_kind: "team", scope_id: TEAM_LONG }
      : { scope_kind: "workspace" },
    20 - index,
  ),
);

function routes(list: () => Response): RouteMap {
  return {
    "GET /me": meRoute({ forecast: ["create"] }),
    "GET /analytics/context": context,
    "GET /forecast/shares": list,
    "DELETE /forecast/shares/share-3": () =>
      new Response(null, { status: 204 }),
  };
}

async function openDrawer(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name: "Shared links" }),
  );
  await screen.findByRole("dialog", { name: "Your shared links" });
}

const meta: Meta<typeof SharedLinksButton> = {
  title: "Records/Reports/Shared links",
  component: SharedLinksButton,
  render: () => (
    <StoryProviders>
      <SharedLinksButton canClose />
    </StoryProviders>
  ),
};
export default meta;

type Story = StoryObj<typeof SharedLinksButton>;

// Three links: a team the picker names, the whole company, and a team this
// reader no longer measures, which reads as its kind rather than its uuid.
export const OpenLinks: Story = {
  beforeEach: () =>
    installFetchStub(routes(() => jsonResponse({ data: THREE }))),
  play: async ({ canvasElement }) => {
    await openDrawer(canvasElement);
    await screen.findByText("Team North");
  },
};

export const NoOpenLinks: Story = {
  beforeEach: () => installFetchStub(routes(() => jsonResponse({ data: [] }))),
  play: async ({ canvasElement }) => {
    await openDrawer(canvasElement);
    await screen.findByText(/You have no open links/);
  },
};

export const ListRefused: Story = {
  beforeEach: () =>
    installFetchStub(
      routes(
        () =>
          new Response(
            JSON.stringify({
              title: "Internal Server Error",
              status: 500,
              detail: "The shared links could not be read.",
            }),
            {
              status: 500,
              headers: { "Content-Type": "application/problem+json" },
            },
          ),
      ),
    ),
  play: async ({ canvasElement }) => {
    await openDrawer(canvasElement);
    await screen.findByRole("alert");
  },
};

// The confirmation every row's Close link opens: one dialog for the list,
// stacked over the drawer so the list is still there behind it.
export const CloseLinkConfirm: Story = {
  beforeEach: () =>
    installFetchStub(routes(() => jsonResponse({ data: THREE }))),
  play: async ({ canvasElement }) => {
    await openDrawer(canvasElement);
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Close link",
        description: /Team North$/,
      }),
    );
    await screen.findByRole("dialog", { name: "Close this link?" });
  },
};

// Fourteen links, half of them for a team whose name runs to two lines: the
// rows scroll inside the drawer and the name wraps rather than widening it.
// The viewport comes from the Storybook manager, so the fe-uat capture of a
// bare iframe is not a phone; review it in Storybook or a narrowed browser.
export const ManyLinksPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  beforeEach: () =>
    installFetchStub(routes(() => jsonResponse({ data: MANY }))),
  play: async ({ canvasElement }) => {
    await openDrawer(canvasElement);
    await screen.findAllByText(/Enterprise accounts/);
  },
};
