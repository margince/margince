/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { formatMoneyCompact } from "../../format/format";
import { en } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MyTargets } from "./analytics.outcomes.targets";
import { reportingPeriodLabel } from "./reporting.model";
import {
  reportingSectionsCatalog,
  reportingStoryRoutes,
  routeSeatEvaluation,
  type SeatFigure,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const SELF = "u-rep-1";

function showTargets(
  figures: Readonly<Record<string, SeatFigure>> | "refused",
  grants: { salesTarget: boolean } = { salesTarget: true },
) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": meRoute({
      report_definition: ["read"],
      reporting_framework: ["read"],
      sales_target: grants.salesTarget ? ["read"] : [],
    }),
    "GET /analytics/metrics": () => jsonResponse(reportingSectionsCatalog),
    "GET /analytics/evaluate": () =>
      jsonResponse({ title: "Unavailable", status: 503 }, 503),
  });
  const asked: URL[] = [];
  if (figures !== "refused") routeSeatEvaluation(figures, asked);
  render(
    <StoryProviders>
      <SessionSettled />
      <MyTargets self={SELF} locale="en" />
    </StoryProviders>,
  );
  return asked;
}

// Shows once the session's grants have arrived, so a case can assert what a
// seat does not see without waiting on a clock.
function SessionSettled() {
  return useCan("report_definition", "read") ? <p>session settled</p> : null;
}

// The cards that name one figure: each one's label, value and detail together.
async function cards(label: string) {
  const names = await screen.findAllByText(label);
  return names.map((name) => {
    const tile = name.closest(".stat-card");
    if (!(tile instanceof HTMLElement)) throw new Error(`${label} has no card`);
    return tile.textContent ?? "";
  });
}

async function card(label: string) {
  const [only, ...more] = await cards(label);
  expect(more).toEqual([]);
  return only ?? "";
}

const progress = (actual: string, target: string) =>
  en["analytics.targetOf"]
    .replace("{actual}", actual)
    .replace("{target}", target);
const share = (percent: string) =>
  en["reporting.attainment"].replace("{percent}", percent);
const QUARTER = reportingPeriodLabel(
  { start_at: "2026-06-30T22:00:00Z", end_at: "2026-09-30T22:00:00Z" },
  REPORTING_FIXTURE_ZONE,
  "en",
);
const MONTH = reportingPeriodLabel(
  { start_at: "2026-08-31T22:00:00Z", end_at: "2026-09-30T22:00:00Z" },
  REPORTING_FIXTURE_ZONE,
  "en",
);

describe("the seat's figures against its own targets", () => {
  it("states progress against a quarter target as a count and a share", async () => {
    showTargets({
      meetings_held: { actual: 10, targets: { fiscal_quarter: 15 } },
    });

    const text = await card(en["reporting.meetings_held"]);
    expect(text).toContain(progress("10", "15"));
    // The dates the target covers, and 10 of 15 told as a whole percent.
    expect(text).toContain(`${QUARTER} · ${share("67")}`);
  });

  it("draws one card per target when a figure has a month and a quarter target", async () => {
    showTargets({
      meetings_held: { actual: 10, targets: { month: 20, fiscal_quarter: 40 } },
    });

    const texts = await cards(en["reporting.meetings_held"]);
    expect(texts).toHaveLength(2);
    expect(texts[0]).toContain(`${QUARTER} · ${share("25")}`);
    expect(texts[1]).toContain(`${MONTH} · ${share("50")}`);
  });

  it("names the failed reading instead of a count against a target", async () => {
    showTargets({ meetings_held: { actual: null, targets: { month: 15 } } });

    const text = await card(en["reporting.meetings_held"]);
    expect(text).toContain(en["reporting.status.failed"]);
    expect(text).not.toContain(progress("0", "15"));
    expect(text).toContain(MONTH);
  });

  it("says a withheld figure is unavailable rather than that no target is set", async () => {
    showTargets({
      meetings_held: { actual: 4, targets: { month: 15 }, withheld: true },
    });

    const text = await card(en["reporting.meetings_held"]);
    expect(text).toContain(en["analytics.readingUnavailable"]);
    expect(text).toContain(en["reporting.withheld"]);
    expect(text).not.toContain(en["analytics.noTargetSet"]);
  });

  it("states a refused reading as an error the reader can retry", async () => {
    showTargets("refused");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      en["common.error"],
    );
    expect(
      screen.getByRole("button", { name: en["common.retry"] }),
    ).toBeTruthy();
  });

  it("says no target is set, and still shows the month so far", async () => {
    showTargets({ bookings_won: { actual: 2_500_000 } });

    const text = await card(en["reporting.bookings_won"]);
    expect(text).toContain(en["analytics.noTargetSet"]);
    expect(text).toContain(
      en["analytics.noTargetSetActual"].replace(
        "{actual}",
        formatMoneyCompact(2_500_000, "EUR", "en"),
      ),
    );
  });

  it("asks for the seat's own figures, per basis, for every targetable metric", async () => {
    const asked = showTargets({});
    await card(en["reporting.meetings_held"]);

    const targetable = reportingSectionsCatalog.metrics
      .filter((metric) => metric.supports_target)
      .map((metric) => metric.id);
    // A lead or partner metric that takes a target joins without a list here.
    expect(targetable).toEqual(
      expect.arrayContaining(["leads_promoted", "partner_pipeline"]),
    );
    expect(
      asked.map((url) => url.searchParams.get("target_basis")).sort(),
    ).toEqual(["fiscal_quarter", "month"]);
    for (const url of asked) {
      expect(url.searchParams.get("scope_kind")).toBe("owner");
      expect(url.searchParams.get("scope_id")).toBe(SELF);
      expect(url.searchParams.getAll("metrics")).toEqual(targetable);
    }
  });

  it("draws nothing for a seat that may not read targets", async () => {
    const asked = showTargets({}, { salesTarget: false });
    await screen.findByText("session settled");

    expect(screen.queryByText(en["analytics.myTargets"])).toBeNull();
    expect(asked).toEqual([]);
  });
});
