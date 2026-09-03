/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ForecastView } from "./analytics.forecast";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The shape the server sends. Every count present, because the gap between
// eligible and priced is what the readings do not cover.
function readings(over: Record<string, unknown> = {}) {
  return {
    period_start: "2026-04-01",
    period_end: "2026-06-30",
    scope_kind: "workspace",
    won_minor: 40_000,
    evidence_minor: 120_000,
    best_case_minor: 180_000,
    open_minor: 250_000,
    weighted_minor: 130_000,
    eligible_count: 12,
    priced_count: 12,
    confirmed_date_count: 10,
    fx_missing_count: 0,
    as_of: "2026-05-14T09:00:00Z",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
    ...over,
  };
}

type StubOpts = {
  readings?: Record<string, unknown>;
  onCall?: (body: Record<string, unknown>) => void;
};

function forecastStub(opts: StubOpts = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (method === "POST" && url.includes("/forecast/calls")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      opts.onCall?.(body);
      return jsonResponse({ id: "c1", ...body }, 201);
    }
    return jsonResponse(opts.readings ?? readings());
  });
}

describe("ForecastView", () => {
  // The answer names BOTH figures. Shown only the call, a reader cannot tell
  // whether it runs ahead of the evidence or behind it — which is the one
  // thing the sentence exists to say.
  it("names the call and what evidence supports, together", async () => {
    vi.stubGlobal(
      "fetch",
      forecastStub({
        readings: readings({
          current_call: {
            id: "c1",
            period_start: "2026-04-01",
            period_end: "2026-06-30",
            scope_kind: "workspace",
            amount_minor: 200_000,
            currency: "EUR",
            author_id: "u1",
            created_at: "2026-05-01T09:00:00Z",
          },
        }),
      }),
    );
    render(<ForecastView />);
    const answer = await screen.findByText(/current call is/i);
    expect(answer.textContent).toContain("2,000.00");
    expect(answer.textContent).toContain("1,200.00");
  });

  // Nobody having called is a real answer, and a different one from a call of
  // zero. It must not render as "the current call is €0".
  it("says nobody has called rather than calling it zero", async () => {
    vi.stubGlobal("fetch", forecastStub());
    render(<ForecastView />);
    expect(await screen.findByText(/Nobody has called/i)).toBeTruthy();
  });

  // An unpriced deal is real pipeline contributing zero money. A total shown
  // without that gap invites the reading where every eligible deal counted.
  it("states the gap when not every deal is priced", async () => {
    vi.stubGlobal(
      "fetch",
      forecastStub({ readings: readings({ priced_count: 9 }) }),
    );
    render(<ForecastView />);
    expect(await screen.findByText(/9 of 12 deals/i)).toBeTruthy();
  });

  it("says nothing about pricing when every deal carries an amount", async () => {
    vi.stubGlobal("fetch", forecastStub());
    render(<ForecastView />);
    await screen.findByText(/Nobody has called/i);
    expect(screen.queryByText(/of 12 deals/i)).toBeNull();
  });

  // The call's amount and note travel as mutation VARIABLES. Read from a
  // closure they would be whatever the last render saw, which is the wrong
  // number exactly when a save races a refetch.
  it("posts the amount and the note the author typed", async () => {
    const posted: Record<string, unknown>[] = [];
    vi.stubGlobal("fetch", forecastStub({ onCall: (b) => posted.push(b) }));
    render(<ForecastView />);

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Update the current call/i }),
    );
    await user.type(
      await screen.findByLabelText(/Supporting note/i),
      "Two renewals slipped",
    );
    await user.click(screen.getByRole("button", { name: /Save call/i }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0].note).toBe("Two renewals slipped");
    expect(posted[0].currency).toBe("EUR");
  });

  // An empty note is NO note. Sent as "", it claims the author wrote something
  // blank, and a reader cannot tell that from a note they cannot see.
  it("sends no note at all when the author wrote none", async () => {
    const posted: Record<string, unknown>[] = [];
    vi.stubGlobal("fetch", forecastStub({ onCall: (b) => posted.push(b) }));
    render(<ForecastView />);

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: /Update the current call/i }),
    );
    await user.click(screen.getByRole("button", { name: /Save call/i }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0].note).toBeUndefined();
  });

  // The receipt is what the total does not cover, beside the total.
  it("shows the counts behind the readings", async () => {
    vi.stubGlobal(
      "fetch",
      forecastStub({ readings: readings({ fx_missing_count: 2 }) }),
    );
    render(<ForecastView />);
    expect(await screen.findByText("Eligible deals")).toBeTruthy();
    expect(screen.getByText("Exchange rate missing")).toBeTruthy();
    expect(screen.getByText("2")).toBeTruthy();
  });
});
