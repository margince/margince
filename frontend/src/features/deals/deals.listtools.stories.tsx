// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, within } from "storybook/test";
import { useMe } from "../../lib/session";
import { replaceParams } from "../../lib/urlstate";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DealsScreen } from "./deals";

// An address can carry a dial this reader may not narrow by. The list names it
// and asks for no deals, rather than show wider rows under the narrowed address.
const meta: Meta = {
  title: "Records/Deals/Withheld filters",
  parameters: { layout: "padded" },
  beforeEach: () => () => globalThis.history.replaceState(null, "", "#/"),
};
export default meta;

type Story = StoryObj;

const stages = [
  { id: "s1", name: "Qualify" },
  { id: "s2", name: "Proposal" },
].map((stage, index) => ({
  ...stage,
  pipeline_id: "pl",
  position: index + 1,
  semantic: "open",
  win_probability: 20,
}));

const deal = {
  id: "d1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

// Every deal list request this story sent, as its query string.
const sent: string[] = [];

// A withheld dial reaching the server narrows to nothing, as the hidden filter
// would for the reader.
function installDealsBackend(
  withheldParameter: string,
  me: { allow: GrantSpec; refusedDealFilters?: string[] },
) {
  sent.length = 0;
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"], ...me })),
    "GET /installation/settings": () => jsonResponse({ base_currency: "EUR" }),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          { id: "pl", name: "Sales", is_default: true, position: 0, stages },
        ],
        page: { next_cursor: null },
      }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: "o1", display_name: "Acme GmbH" }],
        page: { next_cursor: null },
      }),
  });
  const routed = globalThis.fetch;
  const recording: typeof fetch = (input, init) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      request?.url ?? String(input),
      "https://storybook.local",
    );
    const method = request?.method ?? init?.method ?? "GET";
    if (method !== "GET" || url.pathname.replace(/^\/v1/, "") !== "/deals") {
      return routed(input, init);
    }
    sent.push(url.search);
    return Promise.resolve(
      jsonResponse(
        url.searchParams.has(withheldParameter)
          ? emptyPage
          : { data: [deal], page: { next_cursor: null, has_more: false } },
      ),
    );
  };
  globalThis.fetch = recording;
}

// The address a saved view or a shared link would open, a readable stage dial
// beside the withheld one.
function openAddress(withheldParameter: string) {
  globalThis.history.replaceState(null, "", "#/deals");
  replaceParams(
    new Map([
      ["view", "table"],
      ["stage_id", "s1"],
      [withheldParameter, "o1"],
    ]),
  );
}

// The app shell draws a screen once /me has answered, and so does this frame.
function SignedIn({ children }: Readonly<{ children: ReactNode }>) {
  return useMe().data ? children : null;
}

function screen() {
  return (
    <StoryProviders>
      <SignedIn>
        <DealsScreen />
      </SignedIn>
    </StoryProviders>
  );
}

async function expectRefusedDialNamed(
  canvasElement: HTMLElement,
  withheld: { parameter: string; chip: string },
) {
  const canvas = within(canvasElement);
  await expect(
    await canvas.findByText(
      `Your role cannot narrow deals by ${withheld.chip}. Remove that filter to see the list.`,
    ),
  ).toBeTruthy();
  await expect(
    canvas.queryByRole("link", { name: /Fleet retrofit/ }),
  ).toBeNull();
  const chipNames = canvas
    .queryAllByRole("group")
    .map((group) => group.getAttribute("aria-label") ?? "");
  await expect(
    chipNames.filter((name) => name.startsWith(`${withheld.chip}:`)),
  ).toEqual([]);
  await expect(sent).toEqual([]);
}

// A reader without company.read cannot pick a company, so the company dial in
// the address is named as refused.
export const CompanyDialWithoutCompanyRead: Story = {
  beforeEach: () => {
    installDealsBackend("company_id", { allow: { deal: ["read", "update"] } });
    openAddress("company_id");
  },
  render: screen,
  play: async ({ canvasElement }) =>
    expectRefusedDialNamed(canvasElement, {
      parameter: "company_id",
      chip: "Company",
    }),
};

// The server names the partner filter as refused for this role, so the list
// names it and never sends it.
export const PartnerDialTheServerRefuses: Story = {
  beforeEach: () => {
    installDealsBackend("partner_company_id", {
      allow: { deal: ["read", "update"], company: ["read"] },
      refusedDealFilters: ["partner_company_id"],
    });
    openAddress("partner_company_id");
  },
  render: screen,
  play: async ({ canvasElement }) =>
    expectRefusedDialNamed(canvasElement, {
      parameter: "partner_company_id",
      chip: "Partner",
    }),
};
