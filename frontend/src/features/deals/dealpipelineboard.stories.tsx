import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { expect, userEvent, within } from "storybook/test";
import type {
  BoardDeal,
  BoardMoneyColumn,
} from "../../design-system/pipelineboard";
import { WriteToProvider } from "../../lib/writeto";
import { lastEmailAgo } from "../../testing/dealmail";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { StageColumnRead } from "./dealboard.queries";
import { DealPipelineBoard } from "./dealpipelineboard";
import type { Deal, Stage } from "./deals";
import { StageColumnFoot } from "./deals.board";

// The deals board with the screen's half of every card: the verbs, and the
// summary drawer the first of them opens. The cards' own states are canvased in
// design-system/dealcard.stories.tsx; this is where the verbs do something.

const DAY_MS = 24 * 60 * 60 * 1000;

function deal(extra: Partial<BoardDeal> & Pick<BoardDeal, "id" | "name">) {
  return {
    company: "Nordwind Logistik GmbH",
    companyId: "o-1",
    valueMinor: 660_000,
    currency: "EUR",
    idleSince: new Date(
      Date.parse("2026-09-15T12:00:00Z") - 2 * DAY_MS,
    ).toISOString(),
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
    owner: { id: "u-1", name: "Dana Albers" },
    probabilityPct: 40,
    ...extra,
  } satisfies BoardDeal;
}

const proposal: BoardMoneyColumn = {
  stage: "proposal",
  label: "Proposal",
  probabilityPct: 40,
  rawMinor: 18_160_000,
  weightedMinor: 7_264_000,
  currency: "EUR",
  deals: [
    deal({
      id: "d-1",
      name: "deal-de-nordwind-x-gradion-admin-fleetops",
      lastEmail: lastEmailAgo(2 * DAY_MS, "inbound"),
    }),
    deal({
      id: "d-2",
      name: "ERP connector, phase 2",
      company: "Halbach & Söhne KG",
      companyId: "o-2",
      valueMinor: 4_800_000,
      closeDate: "2027-03-14",
      probabilityPct: 75,
      owner: { id: "u-2", name: "Jonas Ritter" },
      lastEmail: lastEmailAgo(5 * 60 * 60 * 1000, "outbound"),
    }),
    deal({
      id: "d-3",
      name: "deal-de-lumenhealth-x-gradion-dataplatform-migration-2026",
      company: "Lumen Health AG",
      companyId: "o-3",
      valueMinor: 12_500_000,
      closeDate: "2027-04-30",
      closeDateProvisional: true,
      stalled: true,
      idleSince: new Date(
        Date.parse("2026-09-15T12:00:00Z") - 23 * DAY_MS,
      ).toISOString(),
      measuredAt: Date.parse("2026-09-15T12:00:00Z"),
      lastEmail: lastEmailAgo(23 * DAY_MS, "outbound"),
    }),
  ],
};

// The pipeline the board's cards stand in, in position order, and the wire row
// behind each card, which is what its menu reads.
const pipelineStages: Stage[] = (
  [
    ["discovery", "Discovery", "open"],
    ["proposal", "Proposal", "open"],
    ["negotiation", "Negotiation", "open"],
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

const cardMenu: ComponentProps<typeof DealPipelineBoard>["menu"] = {
  deals: proposal.deals.map(
    (card): Deal => ({
      id: card.id,
      name: card.name,
      pipeline_id: "sales",
      stage_id: "proposal",
      status: "open",
      source: "manual",
      captured_by: "human:u-1",
      writable: true,
      archivable: true,
      version: 1,
      created_at: "2026-09-01T09:00:00Z",
      updated_at: "2026-09-01T09:00:00Z",
    }),
  ),
  stages: pipelineStages,
  onAdvance: () => undefined,
};

function board() {
  installFetchStub({
    "GET /me": meRoute({
      activity: ["read", "create"],
      deal: ["read", "update", "delete"],
    }),
    "GET /deals/d-1/status": () =>
      jsonResponse({
        deal_id: "d-1",
        story: {
          sentences: [
            {
              text: "Pricing went out last week and the buyer asked for a phased rollout.",
              evidence: [],
            },
          ],
        },
        verdict: {
          standing: "drifting",
          because: {
            sentences: [
              { text: "Nobody has replied since the request.", evidence: [] },
            ],
          },
        },
        generated_at: "2026-09-05T09:00:00Z",
        generated_by: "deterministic",
      }),
    "GET /deals/d-1/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
  });
  return (
    <StoryProviders>
      {/* A mailbox the product can send from, so the mail verb is drawn. */}
      <WriteToProvider writeTo={() => undefined}>
        <DealPipelineBoard
          columns={[proposal]}
          cardHref={(each) => `#/deals/${each.id}`}
          zone="Europe/Berlin"
          menu={cardMenu}
        />
      </WriteToProvider>
    </StoryProviders>
  );
}

const meta: Meta = { title: "Records/Deals/Pipeline board" };
export default meta;
type Story = StoryObj;

export const WithCardVerbs: Story = { render: board };
export const WithCardVerbsDark: Story = {
  globals: { theme: "dark" },
  render: board,
};

const openSummary: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  const [first] = await canvas.findAllByRole("button", {
    name: /^Deal summary:/,
  });
  if (!first) throw new Error("the board drew no deal summary");
  await userEvent.click(first);
  const page = within(canvasElement.ownerDocument.body);
  await expect(
    await page.findByRole("dialog", {
      name: "deal-de-nordwind-x-gradion-admin-fleetops",
    }),
  ).toBeInTheDocument();
};

export const SummaryOpen: Story = { render: board, play: openSummary };
export const SummaryOpenDark: Story = {
  globals: { theme: "dark" },
  render: board,
  play: openSummary,
};

// A card's menu open, with its deal's moves first: the way to move a deal with
// no pointer to drag it by, on a phone as at a desk.
const openCardMenu: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  const trigger = await canvas.findByRole("button", {
    name: "More actions for ERP connector, phase 2",
  });
  await userEvent.click(trigger);
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Move to Negotiation",
    }),
  ).toBeVisible();
};

export const CardMenuOpen: Story = { render: board, play: openCardMenu };
export const CardMenuOpenDark: Story = {
  globals: { theme: "dark" },
  render: board,
  play: openCardMenu,
};
export const CardMenuOpenPhone: Story = {
  tags: ["uat-phone"],
  render: board,
  play: openCardMenu,
};

// A column's own read, as the board's foot draws it under that column's cards.
const settled: StageColumnRead = {
  deals: [],
  isPending: false,
  isError: false,
  error: null,
  refetch: () => undefined,
  hasNextPage: false,
  isFetchingNextPage: false,
  fetchNextPage: () => undefined,
  loadMoreError: null,
};

const discovery: BoardMoneyColumn = {
  stage: "discovery",
  label: "Discovery",
  probabilityPct: 20,
  rawMinor: 2_400_000,
  weightedMinor: 480_000,
  currency: "EUR",
  count: 7,
  deals: [],
};

function columnStates() {
  installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
  const reads = new Map<string, StageColumnRead>([
    [proposal.stage, { ...settled, hasNextPage: true }],
    [
      discovery.stage,
      {
        ...settled,
        isError: true,
        error: new Error("The server could not answer."),
      },
    ],
  ]);
  return (
    <StoryProviders>
      <DealPipelineBoard
        columns={[proposal, discovery]}
        cardHref={(each) => `#/deals/${each.id}`}
        zone="Europe/Berlin"
        columnExtras={(column) => {
          const read = reads.get(column.stage);
          return read && <StageColumnFoot read={read} />;
        }}
      />
    </StoryProviders>
  );
}

// Proposal has more cards to read than it holds; Discovery's own read failed,
// so it says so under its count and offers a retry. Each column pages alone.
const columnStatesPlay: Story["play"] = async ({ canvasElement }) => {
  const proposalColumn = canvasElement.querySelector<HTMLElement>(
    '[data-stage="proposal"]',
  );
  const discoveryColumn = canvasElement.querySelector<HTMLElement>(
    '[data-stage="discovery"]',
  );
  if (!proposalColumn || !discoveryColumn) {
    throw new Error("the board drew no Proposal or Discovery column");
  }
  await expect(
    within(proposalColumn).getByRole("button", { name: "Load more" }),
  ).toBeVisible();
  await expect(within(discoveryColumn).getByRole("alert")).toBeVisible();
  await expect(
    within(discoveryColumn).getByRole("button", { name: "Retry" }),
  ).toBeVisible();
  await expect(
    within(discoveryColumn).queryByRole("button", { name: "Load more" }),
  ).toBeNull();
};

export const ColumnMoreAndFailed: Story = {
  render: columnStates,
  play: columnStatesPlay,
};
export const ColumnMoreAndFailedDark: Story = {
  globals: { theme: "dark" },
  render: columnStates,
  play: columnStatesPlay,
};
