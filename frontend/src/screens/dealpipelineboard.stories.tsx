import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { BoardDeal, BoardMoneyColumn } from "../design-system/composed";
import { DealPipelineBoard } from "./dealpipelineboard";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { WriteToProvider } from "./writeto";

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
    ageMs: 2 * DAY_MS,
    owner: { id: "u-1", name: "Dana Albers" },
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
      lastEmail: { agoMs: 2 * DAY_MS, direction: "inbound" },
    }),
    deal({
      id: "d-2",
      name: "ERP connector, phase 2",
      company: "Halbach & Söhne KG",
      companyId: "o-2",
      valueMinor: 4_800_000,
      closeDate: "2027-03-14",
      owner: { id: "u-2", name: "Jonas Ritter" },
      lastEmail: { agoMs: 5 * 60 * 60 * 1000, direction: "outbound" },
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
      ageMs: 23 * DAY_MS,
      lastEmail: { agoMs: 23 * DAY_MS, direction: "outbound" },
    }),
  ],
};

function board() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create"] }),
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
