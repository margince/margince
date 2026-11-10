// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { MarginceChangeBundle } from "../lib/marginchanges";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { BundleUndo } from "./marginchanges";

// The Undo of several deals Margince changed, before the press and after a
// press the server answered deal by deal.
const meta: Meta<typeof BundleUndo> = {
  title: "Patterns/Margince changes undo",
  component: BundleUndo,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof BundleUndo>;

const deal = (n: number) => `01a00000-0000-7000-8000-0000000005${10 + n}`;

const bundle = (n: number, name: string): MarginceChangeBundle => ({
  deal_id: deal(n),
  deal_name: name,
  version: 7,
  audit_log_ids: [`01a00000-0000-7000-8000-0000000006${10 + n}`],
  kinds: ["close_lost"],
  latest_at: "2026-09-13T02:00:00Z",
});

const BUNDLES = [
  bundle(1, "Acme — ERP implementation"),
  bundle(2, "Nordlicht Logistik — Warehouse management system"),
  bundle(3, "PIM Rollout"),
  bundle(4, "Depot rollout"),
];

// One deal undone, and one of each way a deal is not. One is partly undone,
// one was kept because it changed since, and one archive needs confirming.
const ANSWER = {
  results: [
    {
      deal_id: deal(1),
      outcome: "undone",
      undone: ["close_lost"],
      kept: [],
    },
    {
      deal_id: deal(2),
      outcome: "partly_undone",
      undone: ["close_lost"],
      kept: [
        {
          audit_log_id: "01a00000-0000-7000-8000-000000000701",
          kind: "deal_amount",
          reason: "changed_since",
        },
      ],
    },
    {
      deal_id: deal(3),
      outcome: "refused",
      undone: [],
      kept: [
        {
          audit_log_id: "01a00000-0000-7000-8000-000000000702",
          kind: "close_lost",
          reason: "changed_since",
        },
      ],
    },
    {
      deal_id: deal(4),
      outcome: "refused",
      undone: [],
      kept: [
        {
          audit_log_id: "01a00000-0000-7000-8000-000000000703",
          kind: "deal_open",
          reason: "needs_confirmation",
          team_notes: 1,
          team_tasks: 0,
          team_work: "w",
        },
      ],
    },
  ],
};

function frame() {
  stubWithSession(
    { "POST /deal-corrections/undo": () => jsonResponse(ANSWER) },
    { deal: ["read", "update"] },
  );
  return (
    <StoryProviders>
      <BundleUndo bundles={BUNDLES} label="Undo changes on 4 deals" />
    </StoryProviders>
  );
}

/** Before the press: one Undo for every deal in the list. */
export const Ready: Story = { render: frame };

/**
 * After the press: the counts, then each deal not fully undone, linked, with
 * what happened to it. The undone deal is not listed.
 */
export const UnfinishedDeals: Story = {
  render: frame,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Undo changes on 4 deals" }),
    );
    const list = await canvas.findByRole("list", {
      name: "Deals not fully undone",
    });
    const rows = within(list).getAllByRole("listitem");
    await expect(rows).toHaveLength(3);
    await expect(
      within(list).getByRole("link", { name: /Nordlicht Logistik/ }),
    ).toBeVisible();
    await expect(
      within(list).getByRole("link", { name: "PIM Rollout" }),
    ).toBeVisible();
    await expect(
      within(list).getByRole("link", { name: "Depot rollout" }),
    ).toBeVisible();
    await expect(
      within(list).getByText(/Open the deal to confirm its Undo\./),
    ).toBeVisible();
    await expect(within(list).queryByRole("link", { name: /Acme/ })).toBeNull();
  },
};

export const UnfinishedDealsDark: Story = {
  ...UnfinishedDeals,
  globals: { theme: "dark" },
};

export const UnfinishedDealsPhone: Story = {
  ...UnfinishedDeals,
  tags: ["uat-phone"],
};
