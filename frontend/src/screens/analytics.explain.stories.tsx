// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { CellExplain, ExplainFrame } from "./analytics.explain";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// One row's "Explain this number" drawer, opened from its trigger the way a
// reader opens it. The body is the same one the report card's panel draws, so
// each state here is also that panel's state; the drawer is the host with the
// least room, which is why the states are shown in it.

const HANDLE =
  "/v1/reports/pipeline-current/derivation?by=stage_id&agg=sum:amount_base_minor:raw_minor&stage_id=pl-s1";

const derivation = {
  report: "pipeline-current",
  definition:
    "Sum of open-deal amounts in the base currency, grouped by stage, in Qualify",
  plan: {},
  columns: ["label", "amount_base_minor"],
  rows: [
    { label: "BÄR Pharma, Packaging QA", amount_base_minor: 12343 },
    { label: "Brandt, Line QA Retrofit", amount_base_minor: 12343 },
  ],
  total_rows: 2,
  as_of: "2026-03-04T09:00:00Z",
  as_of_pinned: true,
};

function drawerStory(answer: RouteMap[string]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /reports/pipeline-current/derivation": answer,
    });
    return (
      <StoryProviders>
        {/* No zone: a zone is the server's to name, and the as-of caption it
            frames is drawn in the report stories, whose stub carries one. */}
        <ExplainFrame frame={{ baseCurrency: "EUR", timezone: null }}>
          <CellExplain url={HANDLE} figure="Qualify">
            Qualify
          </CellExplain>
        </ExplainFrame>
      </StoryProviders>
    );
  };
}

// Open the drawer and wait for it: the trigger first, because the drawer is
// portalled out of the canvas and exists only once it has been pressed.
async function openDrawer({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Explain Qualify",
    }),
  );
  await screen.findByRole("dialog");
}

const meta: Meta = { title: "Records/Reports/Explain a cell" };
export default meta;

type Story = StoryObj;

// The ordinary answer: the definition, the rows it reconciles to, the frame.
export const DrawerOpen: Story = {
  render: drawerStory(() => jsonResponse(derivation)),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByText("BÄR Pharma, Packaging QA");
  },
};

// A field mask took records out of the figure and the rows alike; the drawer
// says how many, so a smaller number reads as governed rather than missing.
export const ExcludedByPermission: Story = {
  render: drawerStory(() =>
    jsonResponse({ ...derivation, excluded_by_permission: 3 }),
  ),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByText(/3 records are left out/);
  },
};

// A link that pinned no instant: the figures were recalculated now, and the
// caveat sits above the rows a reader would otherwise take as the headline's.
export const Stale: Story = {
  render: drawerStory(() =>
    jsonResponse({ ...derivation, as_of_pinned: false }),
  ),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByText(/recalculated now/);
  },
};

export const Loading: Story = {
  render: drawerStory(() => new Promise<Response>(() => {})),
  play: openDrawer,
};

export const Failed: Story = {
  render: drawerStory(() =>
    jsonResponse({ title: "Server error", status: 500 }, 500),
  ),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByRole("button", { name: "Retry" });
  },
};

// The figure resolved to no source rows at all.
export const Empty: Story = {
  render: drawerStory(() =>
    jsonResponse({ ...derivation, rows: [], total_rows: 0 }),
  ),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByText("Nothing here yet.");
  },
};

// Forty long names against a server cap: the table scrolls inside the drawer,
// and the rows the cap left off are counted under it.
const longRows = Array.from({ length: 40 }, (_, index) => ({
  label: `Nordhessische Verpackungs- und Logistikgesellschaft, Rahmenvertrag Linienautomatisierung Werk ${index + 1}`,
  amount_base_minor: 1_250_000 + index * 12_345,
}));

export const LongContent: Story = {
  render: drawerStory(() =>
    jsonResponse({ ...derivation, rows: longRows, total_rows: 1240 }),
  ),
  play: async (context) => {
    await openDrawer(context);
    await screen.findByText("1,200 more not shown");
  },
};

// At 390px the drawer is the full-screen sheet; the long rows scroll inside
// their own table rather than pushing the sheet sideways.
export const LongContentPhone: Story = {
  ...LongContent,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
