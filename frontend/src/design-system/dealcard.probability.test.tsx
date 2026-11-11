/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { DealCard } from "./dealcard";
import { armHoverIntent, takeHoverClock } from "./hoverintent-testing";
import type { BoardDeal } from "./pipelineboard";

// The ring on a card says why the deal has its figure on hover, and a press
// on it still opens the deal.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const REASON = "This is the figure the stage Proposal gives.";

function mount(reason?: string) {
  const deal: BoardDeal = {
    id: "d1",
    name: "ERP connector",
    company: "",
    companyId: "",
    valueMinor: 100_000,
    currency: "EUR",
    idleSince: "2026-09-15T12:00:00Z",
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
    probabilityPct: 50,
    probabilityReason: reason,
  };
  const opened = vi.fn((event: React.MouseEvent) => event.preventDefault());
  render(
    <LocaleProvider initial="en">
      <DealCard
        deal={deal}
        href="#/deals/d1"
        zone={browserZone()}
        onOpen={(_, event) => opened(event)}
      />
    </LocaleProvider>,
  );
  return opened;
}

function ring(): HTMLElement {
  const figure = screen.getByText("50%", { exact: false });
  const anchor = figure.closest<HTMLElement>(".deal-probability");
  if (!anchor) {
    throw new Error("the ring carries no reason");
  }
  return anchor;
}

it("shows the reason when the pointer settles on the ring", () => {
  mount(REASON);
  takeHoverClock();
  armHoverIntent();
  fireEvent.pointerEnter(ring());
  act(() => {
    vi.advanceTimersByTime(500);
  });
  expect(screen.getByRole("tooltip")).toHaveTextContent(REASON);
});

it("opens the deal from a press on the ring", () => {
  const opened = mount(REASON);
  fireEvent.click(ring());
  expect(opened).toHaveBeenCalledTimes(1);
});

it("draws the bare ring when there is no reason", () => {
  mount();
  expect(
    screen.getByText("50%", { exact: false }).closest(".deal-probability"),
  ).toBeNull();
});
