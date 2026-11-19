/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import ages from "../../../backend/internal/shared/kernel/elapsed/testdata/ages.json";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { Button } from "./button";
import { DealCard } from "./dealcard";
import type { BoardDeal } from "./pipelineboard";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-06-10T12:00:00Z"));
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function closes(closeDate: string): string {
  const deal: BoardDeal = {
    id: "d1",
    name: "ERP connector",
    company: "",
    companyId: "",
    valueMinor: 100_000,
    currency: "EUR",
    idleSince: "2026-09-15T12:00:00Z",
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
    closeDate,
  };
  render(
    <LocaleProvider initial="en">
      <DealCard deal={deal} href="#/deals/d1" zone={browserZone()} />
    </LocaleProvider>,
  );
  return screen.getByText(/^closes /).textContent ?? "";
}

describe("DealCard close date", () => {
  it("names the year of a close date in another year", () => {
    expect(closes("2027-03-04")).toBe("closes 4 Mar 2027");
  });

  it("names the year of a close date already behind us in a past year", () => {
    expect(closes("2025-12-30")).toBe("closes 30 Dec 2025");
  });

  it("leaves the year off a close date in this year", () => {
    expect(closes("2026-11-03")).toBe("closes 3 Nov");
  });
});

describe("DealCard on a closed deal", () => {
  function closedCard(closedAt: string | null): string {
    render(
      <LocaleProvider initial="en">
        <DealCard
          deal={{
            id: "d1",
            name: "ERP connector",
            company: "",
            companyId: "",
            valueMinor: 100_000,
            currency: "EUR",
            idleSince: "2026-05-15T12:00:00Z",
            measuredAt: Date.parse("2026-06-10T12:00:00Z"),
            closedAt,
          }}
          href="#/deals/d1"
          zone="UTC"
        />
      </LocaleProvider>,
    );
    return document.querySelector(".deal-closes")?.textContent ?? "";
  }

  it("says the day it closed", () => {
    expect(closedCard("2026-05-13T12:00:00Z")).toBe("closed 13 May");
  });

  it("names the year of a close in another year", () => {
    expect(closedCard("2020-01-01T12:00:00Z")).toBe("closed 1 Jan 2020");
  });

  it("claims no day an imported close does not carry", () => {
    expect(closedCard(null)).toBe("no close date");
  });
});

describe("deal ages agree with the server calendar", () => {
  it.each(ages)("$name", ({ at, now, zone, days }) => {
    const measuredAt = Date.parse(now);
    const deal: BoardDeal = {
      id: "d1",
      name: "Company A expansion",
      company: "",
      companyId: "",
      valueMinor: null,
      currency: null,
      idleSince: at,
      measuredAt,
      stalled: true,
      lastEmail: { at, measuredAt, precision: "instant", direction: "inbound" },
    };
    const { container } = render(
      <LocaleProvider initial="en">
        <DealCard deal={deal} href="#/deals/d1" zone={zone} />
      </LocaleProvider>,
    );
    if (days > 0) {
      expect(container.querySelector(".deal-age")).toHaveTextContent(
        `${days}d`,
      );
      expect(screen.getByText(`${days} d ago`)).toBeInTheDocument();
    } else {
      expect(container.querySelector(".deal-age")).toHaveTextContent(
        at === "2026-10-24T22:00:00Z" ? "24h" : "0h",
      );
      expect(screen.queryByText(/d ago$/)).not.toBeInTheDocument();
    }
  });
});

describe("DealCard menu", () => {
  const quiet: BoardDeal = {
    id: "d1",
    name: "ERP connector",
    company: "",
    companyId: "",
    valueMinor: 100_000,
    currency: "EUR",
    idleSince: "2026-06-01T12:00:00Z",
    measuredAt: Date.parse("2026-06-10T12:00:00Z"),
  };
  const onOpen = vi.fn();
  const onCopy = vi.fn();
  // A press on the trigger never settles under the fake clock above.
  beforeEach(() => {
    vi.useRealTimers();
  });
  afterEach(() => {
    onOpen.mockReset();
    onCopy.mockReset();
  });

  function renderCard(deal: BoardDeal) {
    return render(
      <LocaleProvider initial="en">
        <DealCard
          deal={deal}
          href="#/deals/d1"
          zone={browserZone()}
          onOpen={onOpen}
          actions={{
            onSummary: () => undefined,
            menu: <Button onClick={onCopy}>Copy link</Button>,
          }}
        />
      </LocaleProvider>,
    );
  }

  it("closes the verb row with a menu named for its deal", () => {
    const { container } = renderCard(quiet);
    const verbs = container.querySelector<HTMLElement>(".deal-actions");
    if (!verbs) throw new Error("the card drew no verb row");
    const buttons = within(verbs).getAllByRole("button");
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Deal summary: ERP connector",
      "More actions for ERP connector",
    ]);
  });

  it("draws its foot for the menu alone", () => {
    render(
      <LocaleProvider initial="en">
        <DealCard
          deal={quiet}
          href="#/deals/d1"
          zone={browserZone()}
          actions={{ menu: <Button>Copy link</Button> }}
        />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("button", { name: "More actions for ERP connector" }),
    ).toBeInTheDocument();
  });

  it("offers a staged card no menu", () => {
    renderCard({ ...quiet, staged: true });
    expect(
      screen.queryByRole("button", { name: /^More actions for/ }),
    ).toBeNull();
  });

  it("opens its items and runs one without opening the deal", async () => {
    const user = userEvent.setup();
    renderCard(quiet);
    const trigger = screen.getByRole("button", {
      name: "More actions for ERP connector",
    });
    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    await user.click(screen.getByRole("button", { name: "Copy link" }));
    expect(onCopy).toHaveBeenCalledTimes(1);
    expect(onOpen).not.toHaveBeenCalled();
    expect(window.location.hash).toBe("");
  });
});
