/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ForecastDeals } from "./analytics.forecast.deals";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function contribution(over: Record<string, unknown> = {}) {
  return {
    deal_id: "018f0000-0000-7000-8000-000000000001",
    deal_name: "Acme renewal",
    base_minor: 1_000_000,
    weighted_minor: 600_000,
    in_won: false,
    in_open: true,
    in_evidence: false,
    in_best_case: false,
    typed_probability: 20,
    chance_in_use: 60,
    evidence_n: 20,
    signals: ["no_next_step"],
    applied_probability: 60,
    ...over,
  };
}

function serve(body: Record<string, unknown>) {
  const fetchMock = vi.fn(
    async (_request: Request) =>
      new Response(
        JSON.stringify({
          period_start: "2026-10-01",
          period_end: "2026-12-31",
          scope_kind: "workspace",
          won_minor: 0,
          evidence_minor: 0,
          best_case_minor: 0,
          open_minor: 1_000_000,
          weighted_minor: 600_000,
          eligible_count: 1,
          priced_count: 1,
          confirmed_date_count: 1,
          fx_missing_count: 0,
          as_of: "2026-10-31T12:00:00Z",
          timezone: "UTC",
          base_currency: "EUR",
          ...body,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function openDeals() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ForecastDeals
          scope={{ kind: "workspace", label: "Whole company" }}
          period="quarter"
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByText("Show each open deal and its chance"));
}

describe("ForecastDeals", () => {
  it("says how each open deal's weight was made", async () => {
    const fetchMock = serve({
      self_correcting: true,
      contributions: [
        contribution(),
        contribution({
          deal_id: "018f0000-0000-7000-8000-000000000002",
          deal_name: "Already won",
          in_open: false,
          in_won: true,
        }),
      ],
    });
    await openDeals();
    expect(
      await screen.findByText("Typed 20% · 20 recorded outcomes · uses 60%"),
    ).toBeTruthy();
    expect(screen.getByText("No next step")).toBeTruthy();
    expect(screen.queryByText("Already won")).toBeNull();
    expect(fetchMock.mock.calls[0]?.[0].url).toContain("contributions=true");
  });

  it("names the risk-adjusted chance only when an effect moved it", async () => {
    serve({
      self_correcting: true,
      contributions: [contribution({ effect: 0.5, applied_probability: 43 })],
    });
    await openDeals();
    expect(
      await screen.findByText(
        "Typed 20% · 20 recorded outcomes · uses 60% · 43% after risk",
      ),
    ).toBeTruthy();
  });

  it("says the typed chance is used while self-correction is off", async () => {
    serve({
      self_correcting: false,
      contributions: [
        contribution({
          chance_in_use: 20,
          applied_probability: 20,
          evidence_n: undefined,
          signals: [],
          weighted_minor: 200_000,
        }),
      ],
    });
    await openDeals();
    expect(
      await screen.findByText(
        "Each deal counts at the chance typed on its stage. Self-correction is off.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Typed 20% · uses 20%")).toBeTruthy();
  });
});
