// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLayoutEffect, useRef } from "react";
import { Button, SegmentedControl } from "./atoms";
import {
  type BoardDeal,
  type BoardMoneyColumn,
  PipelineBoard,
} from "./composed";
import { ListSurface } from "./listsurface";
import { Select } from "./select";

const meta: Meta<typeof PipelineBoard> = {
  title: "Components/Text and data display/Pipeline board",
  component: PipelineBoard,
};
export default meta;

type Story = StoryObj<typeof PipelineBoard>;

const DAY_MS = 24 * 60 * 60 * 1000;

function boardDeal(
  id: string,
  name: string,
  valueMinor: number | null,
  ageDays: number,
  extra?: Partial<BoardDeal>,
): BoardDeal {
  return {
    id,
    name,
    company: "Acme GmbH",
    companyId: "o-1",
    valueMinor,
    currency: "EUR",
    ageMs: ageDays * DAY_MS,
    ...extra,
  };
}

// Proposal holds no deals: the honest empty column between a lead qualifying
// and the next one reaching it.
const boardColumns: BoardMoneyColumn[] = [
  {
    stage: "discovery",
    label: "Discovery",
    probabilityPct: 10,
    rawMinor: 45_000,
    weightedMinor: 4_500,
    currency: "EUR",
    deals: [
      boardDeal("d1", "Contoso renewal", 12_000, 3, {
        singleThreaded: true,
        closeDate: "2026-10-14",
        owner: { id: "u-1", name: "Ada Lindqvist" },
        lastEmail: { agoMs: 2 * DAY_MS, direction: "inbound" },
      }),
      // A close date the nightly run set and nobody confirmed: marked, not
      // hidden.
      boardDeal("d2", "Fabrikam expansion", 33_000, 9, {
        stalled: true,
        closeDate: "2026-09-30",
        closeDateProvisional: true,
        lastEmail: { agoMs: 10 * DAY_MS, direction: "outbound" },
      }),
    ],
  },
  {
    stage: "qualified",
    label: "Qualified",
    probabilityPct: 30,
    rawMinor: 28_000,
    weightedMinor: 8_400,
    currency: "EUR",
    deals: [
      boardDeal("d3", "Globex onboarding", 28_000, 14, {
        owner: { id: "u-2", name: "Tim Rasche" },
      }),
    ],
  },
  {
    stage: "proposal",
    label: "Proposal",
    probabilityPct: 60,
    rawMinor: 0,
    weightedMinor: 0,
    currency: "EUR",
    deals: [],
  },
  {
    stage: "negotiation",
    label: "Negotiation",
    probabilityPct: 80,
    rawMinor: 54_000,
    weightedMinor: 43_200,
    currency: "EUR",
    deals: [
      boardDeal("d4", "Initech upgrade", 54_000, 21, {
        singleThreaded: true,
      }),
    ],
  },
  {
    stage: "won",
    label: "Closed Won",
    probabilityPct: 100,
    rawMinor: 61_000,
    weightedMinor: 61_000,
    currency: "EUR",
    deals: [boardDeal("d5", "Umbrella Corp", 61_000, 2)],
  },
];

// Inside the same ListSurface a record table renders into, so the header,
// count and tools row read as a table's would.
export const InSurface: Story = {
  render: () => (
    <ListSurface
      count="5 deals"
      search={{ value: "", onChange: () => undefined }}
      action={<Button>New deal</Button>}
      tools={
        <>
          <SegmentedControl
            options={["board", "table"] as const}
            value="board"
            onChange={() => undefined}
            labels={{ board: "Board", table: "Table" }}
          />
          <Select
            className="input"
            aria-label="Pipeline"
            value="sales"
            onChange={() => undefined}
            options={[
              { value: "sales", label: "Sales" },
              { value: "partner", label: "Partner" },
            ]}
          />
        </>
      }
      chips={[
        {
          key: "stage_id",
          label: "Stage",
          allLabel: "All stages",
          options: [
            { value: "discovery", label: "Discovery" },
            { value: "qualified", label: "Qualified" },
          ],
        },
        {
          key: "company_id",
          label: "Company",
          allLabel: "All companies",
          options: [{ value: "acme", label: "Acme GmbH" }],
        },
      ]}
    >
      <PipelineBoard
        columns={boardColumns}
        cardHref={(d) => `#/deals/${d.id}`}
        zone="Europe/Berlin"
      />
    </ListSurface>
  ),
};

// Every absent figure draws as the em dash: a zero is a figure the server never
// sent, and a guessed EUR cannot be told from a real one.
const absentMoneyColumns: BoardMoneyColumn[] = [
  {
    stage: "unpriced",
    label: "Unpriced",
    probabilityPct: 10,
    rawMinor: null,
    weightedMinor: null,
    currency: null,
    count: 3,
    deals: [
      boardDeal("a1", "Contoso pilot", null, 4, { currency: null }),
      boardDeal("a2", "Fabrikam trial", 9_000, 11, { currency: null }),
      boardDeal("a3", "Initech scoping", null, 26, {
        stalled: true,
        currency: "EUR",
      }),
    ],
  },
  {
    stage: "mixed",
    label: "Mixed currencies",
    probabilityPct: 40,
    rawMinor: null,
    weightedMinor: null,
    currency: null,
    sumHidden: true,
    count: 2,
    deals: [
      boardDeal("a4", "Globex EU", 28_000, 6),
      boardDeal("a5", "Globex US", 31_000, 8, { currency: "USD" }),
    ],
  },
  {
    // Withheld for the caller's reason, which the mixed-currency sentence
    // would misstate.
    stage: "withheld",
    label: "Total withheld",
    probabilityPct: 50,
    rawMinor: null,
    weightedMinor: null,
    currency: null,
    sumHidden: true,
    sumHiddenReason: "Loaded deals only. No total while a tag filter is on.",
    count: 2,
    deals: [
      boardDeal("a6", "Northwind renewal", 12_000, 3),
      boardDeal("a7", "Tailspin expansion", 45_000, 9),
    ],
  },
  {
    stage: "loading",
    label: "Totals in flight",
    probabilityPct: 60,
    rawMinor: null,
    weightedMinor: null,
    currency: null,
    deals: [boardDeal("a6", "Umbrella renewal", 54_000, 2)],
  },
];

export const WithAbsentMoney: Story = {
  render: () => (
    <PipelineBoard
      columns={absentMoneyColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// Top to bottom: a company the reader may see, one the payload withheld (the
// mask, never a monogram), and a deal linked to no company.
const withheldCompanyColumns: BoardMoneyColumn[] = [
  {
    stage: "discovery",
    label: "Discovery",
    probabilityPct: 10,
    rawMinor: 99_000,
    weightedMinor: 9_900,
    currency: "EUR",
    deals: [
      boardDeal("w1", "Contoso renewal", 12_000, 3),
      boardDeal("w2", "Fabrikam expansion", 33_000, 9, {
        company: "",
        companyWithheld: true,
      }),
      boardDeal("w3", "Inbound, unlinked", 54_000, 5, { company: "" }),
    ],
  },
];

export const WithWithheldCompany: Story = {
  render: () => (
    <PipelineBoard
      columns={withheldCompanyColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// The name truncates and the count beside it never does: the count is the
// figure a reader compares across the board.
const longStageColumns: BoardMoneyColumn[] = [
  {
    stage: "negotiation",
    label: "Contract negotiation & legal review",
    probabilityPct: 75,
    rawMinor: 412_000,
    weightedMinor: 309_000,
    currency: "EUR",
    deals: [
      boardDeal("l1", "Halbach Werke retrofit", 212_000, 4),
      boardDeal("l2", "Ostmann line upgrade", 200_000, 6),
    ],
  },
  {
    stage: "won",
    label: "Won",
    probabilityPct: 100,
    rawMinor: 96_000,
    weightedMinor: 96_000,
    currency: "EUR",
    deals: [boardDeal("l3", "Riverty rollout", 96_000, 2)],
  },
];

export const WithALongStageName: Story = {
  render: () => (
    <PipelineBoard
      columns={longStageColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// Three stages, so the fold at the pipeline's terminal end stays inside the
// captured frame, beside the open stages it hands its width to.
const foldedStageColumns: BoardMoneyColumn[] = [
  {
    stage: "negotiation",
    label: "Negotiation",
    probabilityPct: 80,
    rawMinor: 54_000,
    weightedMinor: 43_200,
    currency: "EUR",
    deals: [boardDeal("f1", "Initech upgrade", 54_000, 21)],
  },
  {
    stage: "won",
    label: "Closed Won",
    probabilityPct: 100,
    rawMinor: 61_000,
    weightedMinor: 61_000,
    currency: "EUR",
    deals: [boardDeal("f2", "Umbrella Corp", 61_000, 2)],
  },
  {
    // The count comes from the stage: a folded column draws no cards to count.
    stage: "lost",
    label: "Closed Lost",
    probabilityPct: 0,
    rawMinor: 18_000,
    weightedMinor: 0,
    currency: "EUR",
    count: 8,
    collapsed: true,
    deals: [],
  },
];

export const WithAFoldedStage: Story = {
  render: () => (
    <PipelineBoard
      columns={foldedStageColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
      onToggleColumn={() => undefined}
    />
  ),
};

// A card passing under the head is the only thing that shows the band seals
// the column; at rest the two share one ground.
const scrollingStageColumns: BoardMoneyColumn[] = [
  {
    stage: "discovery",
    label: "Discovery",
    probabilityPct: 10,
    rawMinor: 197_000,
    weightedMinor: 19_700,
    currency: "EUR",
    deals: [
      boardDeal("s1", "Contoso renewal", 12_000, 3),
      boardDeal("s2", "Fabrikam expansion", 33_000, 9),
      boardDeal("s3", "Globex onboarding", 28_000, 14),
      boardDeal("s4", "Initech upgrade", 54_000, 21),
      boardDeal("s5", "Umbrella Corp", 61_000, 2),
      boardDeal("s6", "Northwind pilot", 9_000, 6),
    ],
  },
  {
    stage: "qualified",
    label: "Qualified",
    probabilityPct: 30,
    rawMinor: 28_000,
    weightedMinor: 8_400,
    currency: "EUR",
    deals: [boardDeal("s7", "Tailspin rollout", 28_000, 11)],
  },
];

// Short enough that six deals overflow; scrolled three cards deep, so one card
// sits halfway under the band.
const FRAME_H_PX = 440;
const SCROLLED_PAST_PX = 340;

function StageMidScroll() {
  const frame = useRef<HTMLDivElement>(null);

  // The board has no prop for a stage's offset, so the story scrolls it.
  useLayoutEffect(() => {
    const stage = frame.current?.querySelector(".board-col");
    if (!(stage instanceof HTMLElement)) {
      throw new Error(
        "this frame drew no stage, so there is nothing to scroll",
      );
    }
    stage.scrollTop = SCROLLED_PAST_PX;
    if (stage.scrollTop === 0) {
      throw new Error("the stage did not scroll: the frame left it unbounded");
    }
  }, []);

  return (
    <div ref={frame} style={{ display: "flex", height: FRAME_H_PX }}>
      <ListSurface count="7 deals" action={<Button>New deal</Button>}>
        <PipelineBoard
          columns={scrollingStageColumns}
          cardHref={(d) => `#/deals/${d.id}`}
          zone="Europe/Berlin"
        />
      </ListSurface>
    </div>
  );
}

export const WithAScrolledStage: Story = {
  render: () => <StageMidScroll />,
};

// `uat-phone` drives the capture gate to 390px, which Storybook's own viewport
// never reaches in a bare iframe; `fullscreen` keeps the 2rem frame off it.
export const AtPhoneWidth: Story = {
  ...InSurface,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
