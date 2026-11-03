/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import ages from "../../../backend/internal/shared/kernel/elapsed/testdata/ages.json";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
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
      <DealCard deal={deal} href="#/deals/d1" zone={viewerZone()} />
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
