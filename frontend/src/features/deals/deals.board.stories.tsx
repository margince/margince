// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { useMe } from "../../lib/session";
import { meFixture } from "../../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DealsScreen } from "./deals";

// The deal board inside the list surface it is drawn in. A search is read from
// that list, so the board can say where matches sit that it cannot show.
const meta: Meta = {
  title: "Records/Deals/Board search",
  parameters: { layout: "padded" },
  beforeEach: () => {
    installBoardBackend();
    globalThis.history.replaceState(null, "", "#/deals");
    return () => globalThis.history.replaceState(null, "", "#/");
  },
};
export default meta;

type Story = StoryObj;

// Six stages, so the last starts past the edge of a desktop screen too.
const stages = [
  "Qualify",
  "Discovery",
  "Demo",
  "Proposal",
  "Negotiation",
  "Signed",
].map((name, index) => ({
  id: `s${index + 1}`,
  pipeline_id: "pl",
  name,
  position: index + 1,
  semantic: "open",
  win_probability: 10 + index * 15,
}));

const dealNames = [
  "Harbour lighting",
  "Depot rollout",
  "Fleet retrofit",
  "Northgate framework",
  "Canteen refit",
  "Fraunhofer tender",
];

const deals = dealNames.map((name, index) => ({
  id: `d${index + 1}`,
  name,
  amount_minor: 1_000_000 * (index + 1),
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: `s${index + 1}`,
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
}));

// The deal list answers its own `q`, so the board holds only what matched.
function dealsMatching(url: URL): Response {
  const word = (url.searchParams.get("q") ?? "").toLowerCase();
  return jsonResponse({
    data: deals.filter((deal) => deal.name.toLowerCase().includes(word)),
    page: { next_cursor: null, has_more: false },
  });
}

function installBoardBackend() {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          roles: ["rep"],
          allow: { deal: ["read", "update"], company: ["read"] },
        }),
      ),
    "GET /installation/settings": () => jsonResponse({ base_currency: "EUR" }),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          { id: "pl", name: "Sales", is_default: true, position: 0, stages },
        ],
        page: { next_cursor: null },
      }),
    "POST /reports/deals-by-stage": () =>
      jsonResponse({
        report: "deals-by-stage",
        plan: {},
        columns: [],
        rows: [],
      }),
    "GET /companies": () => jsonResponse(emptyPage),
  });
  const routed = globalThis.fetch;
  const answering: typeof fetch = (input, init) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request?.url ?? String(input),
      "https://storybook.local",
    );
    const method = request?.method ?? init?.method ?? "GET";
    if (method === "GET" && url.pathname.replace(/^\/v1/, "") === "/deals") {
      return Promise.resolve(dealsMatching(url));
    }
    return routed(input, init);
  };
  globalThis.fetch = answering;
}

// The app shell draws a screen once /me has answered, and so does this frame.
function SignedIn({ children }: Readonly<{ children: ReactNode }>) {
  return useMe().data ? children : null;
}

function board() {
  return (
    <StoryProviders>
      <SignedIn>
        <DealsScreen />
      </SignedIn>
    </StoryProviders>
  );
}

async function searchOffScreen(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await expect(
    await canvas.findByRole("link", { name: /Harbour lighting/ }),
  ).toBeTruthy();
  await userEvent.type(
    canvas.getByRole("searchbox", { name: "Search" }),
    "fraunhofer",
  );
  await expect(
    await canvas.findByRole("button", { name: "Go to column" }),
  ).toBeTruthy();
  await expect(canvas.getByText("1 result in Signed")).toBeTruthy();
  const stage = canvasElement.querySelector(".board");
  await waitFor(() => expect(stage?.scrollLeft ?? 0).toBeGreaterThan(0));
}

// A search whose one match sits in the last stage: the board names where it
// is and scrolls that stage in.
export const SearchMatchOffScreen: Story = {
  render: board,
  play: async ({ canvasElement }) => searchOffScreen(canvasElement),
};
