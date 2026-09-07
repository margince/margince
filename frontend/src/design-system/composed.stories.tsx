// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLayoutEffect, useRef } from "react";
import { Button, SegmentedControl } from "./atoms";
import {
  type BoardDeal,
  type BoardMoneyColumn,
  PipelineBoard,
  RecordView,
  type TimelineEntry,
} from "./composed";
import { ListSurface } from "./listsurface";
import { Select } from "./select";

// RecordView's timeline gained an optional per-row `actions` slot (the Reply /
// Relink cluster the 360 screens mount). These stories exercise both shapes:
// rows without an affordance render exactly as before, and rows that carry an
// action node get the right-aligned slot — so a render regression in either
// path is caught here rather than only in the screen that composes it.

const emailEntry: TimelineEntry = {
  id: "a1",
  kind: "email",
  title: "Re: Q3 renewal terms",
  atIso: "2026-07-01T09:12:00Z",
  provenance: { kind: "human", self: true },
};
const meetingEntry: TimelineEntry = {
  id: "a2",
  kind: "meeting",
  title: "Discovery call",
  atIso: "2026-06-24T14:00:00Z",
  provenance: { kind: "agent", agent: "capture" },
};
const noteEntry: TimelineEntry = {
  id: "a3",
  kind: "note",
  title: "Left a voicemail",
  atIso: "2026-06-20T16:30:00Z",
  provenance: { kind: "human", self: true },
};
const baseTimeline: TimelineEntry[] = [emailEntry, meetingEntry, noteEntry];

const meta: Meta<typeof RecordView> = {
  title: "Design System/RecordView",
  component: RecordView,
};
export default meta;

type Story = StoryObj<typeof RecordView>;

// The unchanged shape: no row carries an action, so every entry renders as it
// did before the slot existed.
export const Default: Story = {
  args: {
    name: "Acme GmbH",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: baseTimeline,
  },
};

// A mail row as the timeline actually receives one: the stored body carries the
// From/To preamble capture folds in, the sender's sign-off, and the thread they
// replied to. The row shows the message; the sign-off and the quoted history sit
// behind their own control, because the split that finds them is a heuristic and
// a wrong guess has to stay one click from being visible.
export const MailWithSignatureAndQuote: Story = {
  args: {
    name: "Acme GmbH",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...emailEntry,
        title: "Re: Rollout-Plan",
        body: [
          "From: lena.fischer@acme.de",
          "To: lars@gradion.com",
          "",
          "Hallo,",
          "",
          "der Plan sieht gut aus. Phase 1 ist realistisch, bei Phase 3 haben wir",
          "intern noch Klärungsbedarf mit dem Händlerteam. Details stehen unter",
          "https://acme.de/rollout bereit.",
          "",
          "Dienstag 14 Uhr würde bei uns passen.",
          "",
          "Mit freundlichen Grüßen",
          "Lena Fischer",
          "Acme GmbH · +49 89 123456",
          "",
          "Am 12.08.2026 um 09:14 schrieb Lars Jankowfsky:",
          "> Passt ein Termin nächste Woche für die Detailabstimmung?",
        ].join("\n"),
      },
      meetingEntry,
    ],
  },
};

// The same shape on a note, which must NOT be folded: "Viele Grüße" opens this
// one as ordinary prose, and a quoted line is something a person typed.
export const NoteThatReadsLikeASignOff: Story = {
  args: {
    name: "Acme GmbH",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...noteEntry,
        title: "Nach dem Messegespräch",
        body: [
          "Viele Grüße von der Messe ausgerichtet, Lena war sichtlich erfreut.",
          "> Sie fragte nochmal nach dem Händlerportal.",
        ].join("\n"),
      },
    ],
  },
};

// The new slot: the email row carries a Reply action, the meeting row a Relink
// action, and the note row none — the right-aligned cluster only appears where
// an affordance is supplied.
export const WithRowActions: Story = {
  args: {
    name: "Acme GmbH",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...emailEntry,
        actions: (
          <Button small onClick={() => {}}>
            Reply
          </Button>
        ),
      },
      {
        ...meetingEntry,
        actions: (
          <Button small onClick={() => {}}>
            Relink
          </Button>
        ),
      },
      noteEntry,
    ],
  },
};

// PipelineBoard inside ListSurface (design-system/listsurface.tsx) — the same
// shell the record tables render into, so the board's header, count and tools
// row read exactly as a table's would. Four open stages plus one won stage,
// and Proposal carries no deals — the honest empty-column case a stage sees
// between a lead qualifying and the next one reaching it.
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
    org: "Acme GmbH",
    valueMinor,
    currency: "EUR",
    ageMs: ageDays * 24 * 60 * 60 * 1000,
    ...extra,
  };
}

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
        owner: "Ada Lindqvist",
      }),
      // A close date the nightly run set and nobody confirmed: marked, not hidden.
      boardDeal("d2", "Fabrikam expansion", 33_000, 9, {
        stalled: true,
        closeDate: "2026-09-30",
        closeDateProvisional: true,
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
      boardDeal("d3", "Globex onboarding", 28_000, 14, { owner: "Tim Rasche" }),
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

export const BoardInSurface: StoryObj = {
  render: () => (
    <ListSurface
      count="5 deals"
      search={{ value: "", onChange: () => undefined }}
      action={<Button small>New deal</Button>}
      // The board brings its own controls rather than the table's: which
      // pipeline is shown, and whether it is read as stages or as rows.
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
      // The stage and company filters read as the same chip as any other,
      // even though a pipeline named their options rather than a catalog.
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
          key: "organization_id",
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

// The board with money it does not have.
//
// A money figure is an amount AND its currency, and either half can be missing:
// a deal nobody has priced carries neither, an aggregate over such deals carries
// a currency-less total. Every absence draws as the em dash, because the two
// alternatives both state something false — a zero is a figure the server never
// sent, and a guessed EUR cannot be told apart from a real EUR figure.
//
// Read the three columns as three different silences: the first has no currency
// to state a total in, the second holds two and so states none at all (its count
// is the only honest reading), and the third simply has not answered yet.
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
      // An amount with no currency is the case a fallback hides best: it looks
      // like a real figure until somebody asks which money it is in.
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
    // A sum hidden for a reason of the CALLER's, not the mixed-currency one
    // above. The deals board withholds its totals when its card list and its
    // totals query would measure different populations, and a column that drew
    // the mixed-currency sentence there would state a reason that is not true.
    stage: "withheld",
    label: "Total withheld",
    probabilityPct: 50,
    rawMinor: null,
    weightedMinor: null,
    currency: null,
    sumHidden: true,
    sumHiddenReason: "Loaded only — filter to My deals for the total",
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

export const BoardWithAbsentMoney: StoryObj = {
  render: () => (
    <PipelineBoard
      columns={absentMoneyColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// A company has three readings on a card, and the whole point of the middle one
// is that it cannot be mistaken for the third. Read the column top to bottom: a
// company this reader may see, one the payload withheld from them (the mask,
// the same control the deals table's company cell draws — no words, and no
// monogram, since neither is a fact about a company nobody named), and a deal
// that is linked to no company at all.
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
        org: "",
        orgWithheld: true,
      }),
      boardDeal("w3", "Inbound, unlinked", 54_000, 5, { org: "" }),
    ],
  },
];

export const BoardWithWithheldCompany: StoryObj = {
  render: () => (
    <PipelineBoard
      columns={withheldCompanyColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// A STAGE NAME LONGER THAN ITS COLUMN, which is the case the head is built for:
// the name is workspace data of unbounded length, and the count beside it is
// the figure a reader compares across the board. The name truncates; the count
// never does. Written as one composed string in the truncating span — which is
// how this shipped first — a name like the one below ellipsised the figure
// away, so the rearrangement that moved the count up here to keep it on screen
// quietly gave it back. Read the heads across: every count is legible.
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

export const BoardWithALongStageName: StoryObj = {
  render: () => (
    <PipelineBoard
      columns={longStageColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
    />
  ),
};

// A FOLDED STAGE beside open ones, which is the only shape a fold can be read
// in: it takes the width its name and count need and hands the rest back, and
// "the rest" is only visible next to a stage that kept it.
//
// Three stages rather than the deal board above, because a fold belongs at the
// terminal end of a pipeline and the deal board's right-hand end is past the
// edge of every frame this catalog captures — a folded column nobody can see
// documents nothing. Same reason it is not on the phone story: one stage takes
// four fifths of that width, so anything but the first is off screen.
//
// The head is a real control here (`aria-expanded`, Enter and Space) because
// the column carries `collapsed`; the two open stages stay the plain text they
// have always been, which is the per-column rule the board keeps. Pressing it
// does nothing — the catalog draws states rather than driving them.
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
    // Its count comes from the stage rather than from the cards: a folded
    // column draws none of them, and a figure taken from what is drawn would
    // read zero on every stage a reader folds away.
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

export const BoardWithAFoldedStage: StoryObj = {
  render: () => (
    <PipelineBoard
      columns={foldedStageColumns}
      cardHref={(d) => `#/deals/${d.id}`}
      zone="Europe/Berlin"
      onToggleColumn={() => undefined}
    />
  ),
};

// A STAGE MID-SCROLL, which is the one state that shows what its head is for.
//
// At rest the head is indistinguishable from the column behind it: they carry
// one ground, so whether the band reaches the column's edges or leaves a strip
// of scrollport above and beside it looks identical until something scrolls
// through that strip. A card passing under the head is what tells them apart —
// it goes BEHIND the band and is visible nowhere above or beside it.
//
// Deep enough to overflow, in a frame short enough to overflow it: a column
// takes its `overflow-y: auto` from the list surface (listtable.css) and only
// scrolls when the surface has a bounded height to scroll inside. The second
// stage holds one deal, so the same head reads beside it at rest.
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

// Short enough that six deals do not fit, which is what makes the stage a
// scroller at all; the offset is three cards deep, which catches one of them
// halfway under the band rather than a seam between two.
const FRAME_H_PX = 440;
const SCROLLED_PAST_PX = 340;

function StageMidScroll() {
  const frame = useRef<HTMLDivElement>(null);

  // Scrolled, not passed: the board has no prop for a stage's offset, and the
  // same reasoning the sort-menu story presses its own trigger with — a frame
  // of a stage that never scrolled would picture the very thing this story
  // exists to rule out.
  useLayoutEffect(() => {
    const stage = frame.current?.querySelector(".board-col");
    if (!(stage instanceof HTMLElement)) {
      throw new Error(
        "this frame drew no stage, so there is nothing to scroll",
      );
    }
    stage.scrollTop = SCROLLED_PAST_PX;
    // A frame that does not bound the surface leaves every stage its full
    // height, and the story would then picture a head sealing nothing while
    // reading as though it had.
    if (stage.scrollTop === 0) {
      throw new Error("the stage did not scroll: the frame left it unbounded");
    }
  }, []);

  return (
    <div ref={frame} style={{ display: "flex", height: FRAME_H_PX }}>
      <ListSurface count="7 deals" action={<Button small>New deal</Button>}>
        <PipelineBoard
          columns={scrollingStageColumns}
          cardHref={(d) => `#/deals/${d.id}`}
          zone="Europe/Berlin"
        />
      </ListSurface>
    </div>
  );
}

export const BoardWithAScrolledStage: StoryObj = {
  render: () => <StageMidScroll />,
};

/**
 * The same board on a phone, where a reader swipes between stages instead of
 * seeing them side by side.
 *
 * One stage and the gap after it take four fifths of the board's width, so the
 * next stage shows about a quarter of itself — the peek is the only thing that
 * says the pipeline carries on, and the board snaps so a swipe cannot leave the
 * reader parked between two stages. The frame reuses the deal board above
 * unchanged: the phone shape is the stylesheet's, not a second component's, and
 * a story that built its own columns could agree with the CSS while the real
 * board did not.
 *
 * `uat-phone` is what makes the capture gate drive the browser to 390px —
 * Storybook's own viewport is applied by the manager, which the gate's bare
 * `iframe.html` never runs, so without the tag this would be captured at
 * desktop width and would picture the very layout it exists to rule out.
 * `fullscreen` keeps the catalog's 2rem frame off it: 390px less two frames is
 * not a width any reader has.
 */
export const BoardAtPhoneWidth: StoryObj = {
  ...BoardInSurface,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
