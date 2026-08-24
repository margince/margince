/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import {
  BriefItemCard,
  BriefItemCardPending,
  type BriefItemLabels,
} from "./briefitem";

afterEach(cleanup);

type BriefItem = components["schemas"]["MorningBriefItem"];
type FeatureVector = components["schemas"]["MorningBriefFeatureVector"];

const ITEM_ID = "3f7c1a8e-0000-4000-8000-000000000001";
const DEAL_ID = "9b21d0c4-0000-4000-8000-000000000002";

// Typed as the WIRE vector, deliberately: a factor added to the contract stops
// this object compiling until it is named here, and the bar census below then
// fails until the component's own list carries it too. That is the only thing
// standing between a new dimension and rendering as five bars and a silence.
const VECTOR: FeatureVector = {
  winnability: 0.6,
  revenue: 0.94,
  timing: 0.35,
  momentum: 1,
  warmth: 0.52,
};

const LABELS: BriefItemLabels = {
  rank: "Rank",
  composite: "Score",
  factors: {
    winnability: "Winnability",
    revenue: "Revenue",
    timing: "Timing",
    momentum: "Momentum",
    warmth: "Warmth",
  },
  evidence: "3 sources",
  evidenceNone: "No sources",
  openDeal: "Open deal",
  act: "Act",
  snooze: "Snooze",
  dismiss: "Dismiss",
  acted: "Acted",
  dismissed: "Dismissed",
  snoozed: "Snoozed",
  resurfaces: "Back",
};

const ITEM: BriefItem = {
  id: ITEM_ID,
  deal_id: DEAL_ID,
  rank: 1,
  composite: 0.78,
  feature_vector: VECTOR,
  evidence_ids: ["1a000000-0000-4000-8000-00000000000a"],
  state: "new",
  state_at: null,
  snoozed_until: null,
};

// No clock and no locale database: both formatters are pure functions of their
// argument, so every assertion below is about the card rather than about the
// machine's timezone.
const formatInstant = (utcIso: string) => `at ${utcIso}`;
const formatPercent = (fraction: number) => `${Math.round(fraction * 100)}%`;

type CardProps = Parameters<typeof BriefItemCard>[0];

function cardProps(overrides: Partial<CardProps> = {}): CardProps {
  return {
    item: ITEM,
    labels: LABELS,
    dealName: "Nordwind Logistik",
    formatInstant,
    formatPercent,
    onOpenDeal: () => {},
    onAct: () => {},
    onDismiss: () => {},
    onSnooze: () => {},
    ...overrides,
  };
}

function cardElement(): HTMLElement {
  const card = document.querySelector<HTMLElement>(".brief-item");
  if (!card) {
    throw new Error("the BriefItemCard rendered no card");
  }
  return card;
}

function meter(name: string): HTMLElement {
  return screen.getByRole("meter", { name });
}

describe("BriefItemCard hands every decision back to its caller", () => {
  it("fires act, snooze and dismiss with the item id", async () => {
    const user = userEvent.setup();
    const onAct = vi.fn();
    const onSnooze = vi.fn();
    const onDismiss = vi.fn();
    render(<BriefItemCard {...cardProps({ onAct, onSnooze, onDismiss })} />);

    await user.click(screen.getByRole("button", { name: "Act" }));
    await user.click(screen.getByRole("button", { name: "Snooze" }));
    await user.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(onAct).toHaveBeenCalledWith(ITEM_ID);
    expect(onSnooze).toHaveBeenCalledWith(ITEM_ID);
    expect(onDismiss).toHaveBeenCalledWith(ITEM_ID);
  });

  // The DEAL id, not the item id: the caller is navigating to a deal, and an
  // item id would make every call site look the deal up again to route.
  it("opens the deal the item is about, by deal id", async () => {
    const user = userEvent.setup();
    const onOpenDeal = vi.fn();
    render(<BriefItemCard {...cardProps({ onOpenDeal })} />);

    await user.click(screen.getByRole("button", { name: "Nordwind Logistik" }));

    expect(onOpenDeal).toHaveBeenCalledWith(DEAL_ID);
  });

  // `pending` marks the pressed control busy WITHOUT disabling it — the reader
  // keeps focus where they put it — and refuses the other two outright.
  it("keeps the pressed verb focusable while it refuses the others", async () => {
    const user = userEvent.setup();
    const onAct = vi.fn();
    render(<BriefItemCard {...cardProps({ pending: "act", onAct })} />);

    const act = screen.getByRole("button", { name: "Act" });
    expect(act.getAttribute("aria-busy")).toBe("true");
    expect(act.hasAttribute("disabled")).toBe(false);
    expect(
      screen.getByRole("button", { name: "Dismiss" }).hasAttribute("disabled"),
    ).toBe(true);

    // A second press while the first write is out must not reach the caller.
    await user.click(act);
    expect(onAct).not.toHaveBeenCalled();
  });

  it("announces a refusal the write came back with", () => {
    render(
      <BriefItemCard
        {...cardProps({ error: "This item was already acted on." })}
      />,
    );
    expect(screen.getByRole("alert").textContent).toBe(
      "This item was already acted on.",
    );
  });
});

describe("BriefItemCard draws the composite and its factors on one axis", () => {
  it("draws one bar per factor the contract carries, plus the composite", () => {
    render(<BriefItemCard {...cardProps()} />);
    expect(screen.getAllByRole("meter")).toHaveLength(
      Object.keys(VECTOR).length + 1,
    );
  });

  it("gives every bar the proportion it was handed, as a share of 100", () => {
    render(<BriefItemCard {...cardProps()} />);
    expect(meter("Score").getAttribute("aria-valuenow")).toBe("78");
    expect(meter("Winnability").getAttribute("aria-valuenow")).toBe("60");
    expect(meter("Revenue").getAttribute("aria-valuenow")).toBe("94");
    expect(meter("Timing").getAttribute("aria-valuenow")).toBe("35");
    expect(meter("Momentum").getAttribute("aria-valuenow")).toBe("100");
    expect(meter("Warmth").getAttribute("aria-valuenow")).toBe("52");
    for (const bar of screen.getAllByRole("meter")) {
      expect(bar.getAttribute("aria-valuemax")).toBe("100");
    }
  });

  // The bar and the digits beside it come from the SAME clamped fraction, so a
  // value the scoring should never emit cannot make them disagree.
  it("clamps a value outside 0..1 in the bar and in the figure alike", () => {
    render(
      <BriefItemCard
        {...cardProps({
          item: {
            ...ITEM,
            composite: 1.4,
            // Momentum comes off its 1.0 ceiling so the clamped composite is
            // the ONLY 100% on the card and the assertion names one cell.
            feature_vector: { ...VECTOR, timing: -0.2, momentum: 0.5 },
          },
        })}
      />,
    );
    expect(meter("Score").getAttribute("aria-valuenow")).toBe("100");
    expect(meter("Timing").getAttribute("aria-valuenow")).toBe("0");
    expect(screen.getByText("100%")).toBeTruthy();
    expect(screen.getByText("0%")).toBeTruthy();
  });

  it("names the base the revenue factor normalised against", () => {
    render(
      <BriefItemCard
        {...cardProps({ revenueBasisNote: "normalised against €195,000.00" })}
      />,
    );
    expect(screen.getByText("normalised against €195,000.00")).toBeTruthy();
  });
});

describe("BriefItemCard states what the rep already did", () => {
  it("shows a snoozed item's own state, and when it comes back", () => {
    render(
      <BriefItemCard
        {...cardProps({
          item: {
            ...ITEM,
            state: "snoozed",
            state_at: "2026-08-24T06:12:00Z",
            snoozed_until: "2026-08-27T06:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByText("Snoozed")).toBeTruthy();
    expect(screen.getByText("at 2026-08-24T06:12:00Z")).toBeTruthy();
    expect(screen.getByText("at 2026-08-27T06:00:00Z")).toBeTruthy();
    // The raw instant stays on the element, so the rendered words are the
    // reader's and the machine-readable value is still the wire's.
    expect(
      document.querySelector('time[datetime="2026-08-27T06:00:00Z"]')
        ?.textContent,
    ).toBe("at 2026-08-27T06:00:00Z");
  });

  // A snooze is a deferral, not a verdict: the item comes back, so the rep can
  // still pull it forward.
  it("keeps the verbs on a snoozed item", () => {
    render(
      <BriefItemCard
        {...cardProps({
          item: {
            ...ITEM,
            state: "snoozed",
            snoozed_until: "2026-08-27T06:00:00Z",
          },
        })}
      />,
    );
    expect(screen.getByRole("button", { name: "Act" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Dismiss" })).toBeTruthy();
  });

  it("offers no verbs on an item already acted on", () => {
    render(
      <BriefItemCard
        {...cardProps({
          item: { ...ITEM, state: "acted", state_at: "2026-08-24T07:41:00Z" },
        })}
      />,
    );
    expect(screen.getByText("Acted")).toBeTruthy();
    expect(screen.getByText("at 2026-08-24T07:41:00Z")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Act" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Snooze" })).toBeNull();
  });

  it("offers no verbs on a dismissed item", () => {
    render(
      <BriefItemCard
        {...cardProps({ item: { ...ITEM, state: "dismissed" } })}
      />,
    );
    expect(screen.getByText("Dismissed")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Act" })).toBeNull();
  });

  // The recede is `Card`'s own recessed variant, not an opacity over the whole
  // card. The distinction is the point: `opacity` dimmed the state word and the
  // focus ring along with everything else.
  it("recedes a settled item into the recessed card variant", () => {
    render(
      <BriefItemCard {...cardProps({ item: { ...ITEM, state: "acted" } })} />,
    );
    const card = cardElement();
    expect(card.classList.contains("card-inset")).toBe(true);
    expect(card.classList.contains("brief-item-quiet")).toBe(true);
    expect(card.style.opacity).toBe("");
  });

  it("leaves an unworked item at full weight", () => {
    render(<BriefItemCard {...cardProps()} />);
    const card = cardElement();
    expect(card.classList.contains("card-inset")).toBe(false);
    expect(card.classList.contains("brief-item-quiet")).toBe(false);
  });
});

describe("BriefItemCard is honest about what is behind the score", () => {
  it("carries the caller's already-pluralised evidence count", () => {
    render(<BriefItemCard {...cardProps()} />);
    expect(screen.getByText("3 sources")).toBeTruthy();
    expect(screen.queryByText("No sources")).toBeNull();
  });

  // Evidence-or-omit means a brief item always has sources. An empty list is a
  // fault, and stating it is the difference between a reader distrusting the
  // score and a reader not knowing they should.
  it("says so when an item arrived with no evidence at all", () => {
    render(
      <BriefItemCard {...cardProps({ item: { ...ITEM, evidence_ids: [] } })} />,
    );
    expect(screen.getByText("No sources")).toBeTruthy();
    expect(screen.queryByText("3 sources")).toBeNull();
  });

  it("still opens the deal before the deal's name is read", async () => {
    const user = userEvent.setup();
    const onOpenDeal = vi.fn();
    render(
      <BriefItemCard {...cardProps({ dealName: undefined, onOpenDeal })} />,
    );
    await user.click(screen.getByRole("button", { name: "Open deal" }));
    expect(onOpenDeal).toHaveBeenCalledWith(DEAL_ID);
  });

  it("draws no money line for a deal nobody has priced", () => {
    render(<BriefItemCard {...cardProps({ amount: null })} />);
    expect(document.querySelector(".brief-item-amount")).toBeNull();
  });
});

describe("BriefItemCardPending", () => {
  it("announces the wait for a reader who cannot see the bones", () => {
    render(<BriefItemCardPending label="Reading this morning's brief" />);
    const region = screen.getByRole("status");
    expect(region.getAttribute("aria-busy")).toBe("true");
    expect(region.textContent).toContain("Reading this morning's brief");
  });
});
