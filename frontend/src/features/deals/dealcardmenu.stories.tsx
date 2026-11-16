// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { DealCard } from "../../design-system/dealcard";
import type { BoardDeal } from "../../design-system/pipelineboard";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealCardMenu } from "./dealcardmenu";
import type { Deal, Stage } from "./deals";

// One card's "…" menu in each state a deal can be in, opened on the card.

const stages: Stage[] = (
  [
    ["discovery", "Discovery", "open"],
    ["proposal", "Proposal", "open"],
    ["won", "Won", "won"],
    ["lost", "Lost", "lost"],
  ] as const
).map(([id, name, semantic], position) => ({
  id,
  pipeline_id: "sales",
  name,
  position,
  semantic,
  win_probability: 40,
}));

function wire(extra: Partial<Deal>): Deal {
  return {
    id: "d-1",
    name: "ERP connector, phase 2",
    pipeline_id: "sales",
    stage_id: "discovery",
    status: "open",
    source: "manual",
    captured_by: "human:u-1",
    writable: true,
    archivable: true,
    version: 1,
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    ...extra,
  };
}

function MenuOnCard({ deal }: Readonly<{ deal: Deal }>) {
  installFetchStub({
    "GET /me": meRoute({
      activity: ["read", "create"],
      deal: ["read", "update", "delete"],
    }),
  });
  const archived = Boolean(deal.archived_at);
  const card: BoardDeal = {
    id: deal.id,
    name: deal.name,
    company: "Halbach & Söhne KG",
    companyId: "o-2",
    valueMinor: 4_800_000,
    currency: "EUR",
    idleSince: "2026-09-13T12:00:00Z",
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
    probabilityPct: 75,
    archived,
  };
  const verbs = {
    onSummary: () => undefined,
    onEmail: archived ? undefined : () => undefined,
    onAddTask: archived ? undefined : () => undefined,
  };
  return (
    <StoryProviders>
      <div style={{ width: 300, maxWidth: "100%" }}>
        <DealCard
          deal={card}
          href="#/deals/d-1"
          zone="Europe/Berlin"
          actions={{
            ...verbs,
            menu: (
              <DealCardMenu
                deal={deal}
                stages={stages}
                verbs={verbs}
                onAdvance={() => undefined}
              />
            ),
          }}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = { title: "Records/Deals/Deal card menu" };
export default meta;
type Story = StoryObj;

// Opens the card's menu and waits for an item the story is about.
function openMenuShowing(first: string) {
  return async ({
    canvasElement,
  }: Readonly<{ canvasElement: HTMLElement }>) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "More actions for ERP connector, phase 2",
      }),
    );
    await expect(
      await screen.findByRole("button", { name: first }),
    ).toBeVisible();
  };
}

const openDeal = {
  render: () => <MenuOnCard deal={wire({})} />,
  play: openMenuShowing("Move to Proposal"),
};
export const OpenDeal: Story = openDeal;
export const OpenDealDark: Story = { ...openDeal, globals: { theme: "dark" } };
export const OpenDealPhone: Story = {
  ...openDeal,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A won deal keeps its moves, refused, with the line that says where it reopens.
export const WonDeal: Story = {
  render: () => <MenuOnCard deal={wire({ status: "won", stage_id: "won" })} />,
  play: async (context) => {
    await openMenuShowing("Mark lost")(context);
    await expect(
      screen.getByRole("button", { name: "Mark lost" }),
    ).toBeDisabled();
    await expect(
      screen.getByText(
        "This deal is closed. Reopen it on its page to move it.",
      ),
    ).toBeVisible();
  },
};

export const ArchivedDeal: Story = {
  render: () => (
    <MenuOnCard deal={wire({ archived_at: "2026-09-10T09:00:00Z" })} />
  ),
  play: openMenuShowing("Copy link"),
};
