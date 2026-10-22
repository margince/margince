/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { RecordZoneProvider } from "../../lib/recordzone";
import { zeroWeekCounts } from "../../testing/brief.fixtures";
import {
  fleetDeal,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";

// The weekly report's own shape: its head, its sections, the stepper and the
// last line. What the figures say is in brief.weekly.test.tsx.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  window.location.hash = "";
});

beforeEach(() => {
  window.location.hash = "#/home?view=weekly";
});

// ── The report's own shape ──

describe("BriefScreen — the weekly report", () => {
  const review = {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts: { ...zeroWeekCounts, deals_won: 1 },
    deals: [],
  };

  const openReport = (weeks: readonly string[]) => {
    stubApi({
      "GET /weekly-reviews/latest": () => jsonResponse(review),
      "GET /weekly-reviews": () => jsonResponse({ weeks }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(
      <RecordZoneProvider zone="Europe/Berlin">
        <BriefScreen />
      </RecordZoneProvider>,
    );
  };

  it("titles the report by the week it covers, and names its calendar week", async () => {
    openReport(["2026-06-29"]);

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: /^29 June\s*[–-]\s*5 July$/,
      }),
    ).toBeTruthy();
    expect(
      screen.getByText(en["brief.report.kicker"].replace("{week}", "27")),
    ).toBeTruthy();
  });

  // Each closed deal says how it ended and what it closed at; a lost one's
  // worth is marked as what the week did not get.
  it("lists each closed deal with its outcome and what it closed at", async () => {
    stubApi({
      "GET /weekly-reviews/latest": () =>
        jsonResponse({
          ...review,
          deals: [
            {
              deal_id: "01a04000-0000-7000-8000-0000000000d1",
              label: "Weber Rahmenvertrag",
              outcome: "won",
              amount_minor_at_close: 7200000,
              currency_at_close: "EUR",
              occurred_at: "2026-07-02T10:00:00Z",
            },
            {
              deal_id: "01a04000-0000-7000-8000-0000000000d2",
              label: "Aster Handel",
              outcome: "lost",
              amount_minor_at_close: 3100000,
              currency_at_close: "EUR",
              occurred_at: "2026-07-03T10:00:00Z",
            },
          ],
        }),
      "GET /weekly-reviews": () => jsonResponse({ weeks: ["2026-06-29"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(<BriefScreen />);

    const won = (await screen.findByText("Weber Rahmenvertrag")).closest(
      ".brief-report-deal",
    );
    expect(won?.textContent).toContain(en["brief.weekly.outcome.won"]);
    expect(
      won?.querySelector(".brief-report-deal-amount")?.textContent,
    ).toMatch(/€72,000\.00/);
    const lost = screen.getByText("Aster Handel").closest(".brief-report-deal");
    expect(lost?.textContent).toContain(en["brief.weekly.outcome.lost"]);
    expect(
      lost
        ?.querySelector(".brief-report-deal-amount")
        ?.getAttribute("data-outcome"),
    ).toBe("lost");
  });

  // A section with nothing to say is left out rather than drawn empty.
  it("leaves out a section with nothing to say", async () => {
    openReport(["2026-06-29"]);

    await screen.findByText(en["brief.report.chapter.figures"], {
      selector: "h2",
    });
    // No deal closed this week, so there is no section for closed deals.
    expect(
      screen.queryByRole("heading", {
        name: new RegExp(en["brief.report.chapter.closed"]),
      }),
    ).toBeNull();
  });

  // The stepper reaches only weeks that have a report, and writes the week
  // into the address so the page can be sent on.
  it("steps to the week before, and not past the newest", async () => {
    openReport(["2026-06-29", "2026-06-22"]);
    const user = userEvent.setup();

    const later = await screen.findByRole("button", {
      name: en["brief.report.later"],
    });
    expect(later).toHaveProperty("disabled", true);
    await user.click(
      screen.getByRole("button", { name: en["brief.report.earlier"] }),
    );
    await waitFor(() =>
      expect(window.location.hash).toContain("week=2026-06-22"),
    );
  });

  it("says when the next report comes", async () => {
    openReport(["2026-06-29"]);

    expect(
      await screen.findByText(
        new RegExp(
          `^${en["brief.report.next"].replace("{date}", "Monday,? 13 July")}$`,
        ),
      ),
    ).toBeTruthy();
  });

  // An earlier week's report was followed long ago. The next report is the
  // one after the newest week, whichever week is open.
  it("dates the next report from the newest week, on an earlier one too", async () => {
    stubApi({
      "GET /weekly-reviews/latest": (_body, query) =>
        jsonResponse(
          query.get("week") === "2026-06-22"
            ? { ...review, local_week_start: "2026-06-22" }
            : review,
        ),
      "GET /weekly-reviews": () =>
        jsonResponse({ weeks: ["2026-06-29", "2026-06-22"] }),
      "GET /deals": () => jsonResponse({ data: [fleetDeal] }),
    });
    render(
      <RecordZoneProvider zone="Europe/Berlin">
        <BriefScreen />
      </RecordZoneProvider>,
    );
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: en["brief.report.earlier"] }),
    );
    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: /^22\s*[–-]\s*28 June$/,
      }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        new RegExp(
          `^${en["brief.report.next"].replace("{date}", "Monday,? 13 July")}$`,
        ),
      ),
    ).toBeTruthy();
  });
});
