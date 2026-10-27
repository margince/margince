// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DealScreen } from "./deals.detail";

// The status pill beside the deal's name. It is the one status badge every
// screen draws, so its tone and words match the board and the company page.
const meta = {
  title: "Records/Deal 360/Status pill",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

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

const caller = {
  user: { id: "u-9", display_name: "Me" },
  roles: ["rep"],
  teams: [],
};

function DealPage({ status }: Readonly<{ status: string }>) {
  installFetchStub({
    "GET /deals/d1": () => jsonResponse({ ...deal, status }),
    "GET /deals/d1/offers": () => jsonResponse(emptyPage),
    "GET /deals/d1/stakeholders": () => jsonResponse(emptyPage),
    "GET /pipelines": () => jsonResponse(emptyPage),
    "GET /approvals": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /activities": () => jsonResponse(emptyPage),
    "GET /records/deal/d1/context": () =>
      jsonResponse({ anchor: { type: "deal", id: "d1" }, sections: [] }),
    "GET /me": () => jsonResponse(caller),
  });
  return (
    <StoryProviders>
      <DealScreen id="d1" />
    </StoryProviders>
  );
}

async function expectStatusPill(
  canvasElement: HTMLElement,
  words: string,
  toneClass: string | null,
) {
  await within(canvasElement).findByRole("heading", {
    level: 1,
    name: "Fleet retrofit",
  });
  const pill = canvasElement.querySelector(".record-split-pills .badge");
  await expect(pill?.textContent).toBe(words);
  const tones = [...(pill?.classList ?? [])].filter(
    (name) => name !== "badge" && name.startsWith("badge-"),
  );
  await expect(tones).toEqual(toneClass ? [toneClass] : []);
}

export const Open: Story = {
  name: "Open deal",
  render: () => <DealPage status="open" />,
  play: async ({ canvasElement }) => {
    await expectStatusPill(canvasElement, "Open", null);
  },
};

export const Won: Story = {
  name: "Won deal",
  render: () => <DealPage status="won" />,
  play: async ({ canvasElement }) => {
    await expectStatusPill(canvasElement, "Won", "badge-success");
  },
};

export const Lost: Story = {
  name: "Lost deal",
  render: () => <DealPage status="lost" />,
  play: async ({ canvasElement }) => {
    await expectStatusPill(canvasElement, "Lost", "badge-danger");
  },
};

// A status a newer server sends that this build has no word for.
export const UnknownStatus: Story = {
  name: "Status this build has no word for",
  render: () => <DealPage status="on_hold" />,
  play: async ({ canvasElement }) => {
    await expectStatusPill(canvasElement, "on_hold", null);
  },
};
