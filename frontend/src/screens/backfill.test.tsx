/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { BackfillPanel } from "./backfill";
import { installFetchStub } from "./story-utils";

// The connect-time backfill is the coldstart payoff: the scope must auto-load
// (honest scope before any click), the spend must still wait for the explicit
// start (ADR-0020 preview-before-spend), and the run must render the three
// headline figures — captured mail, contacts, companies — from real persisted
// counts as they climb. Every number here is a server number.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type BackfillPreview = components["schemas"]["BackfillPreview"];

const previewOf = (messages: number): BackfillPreview => ({
  window: "6m",
  estimated_messages: messages,
  computed_at: "2026-07-23T10:00:00Z",
});

const statusNone: BackfillStatus = { state: "none" };

function countsStatus(
  state: BackfillStatus["state"],
  counts: NonNullable<BackfillStatus["counts"]>,
  estimated = 400,
): BackfillStatus {
  return {
    state,
    backfill_id: "018f3a1b-0000-7000-8000-0000000000b1",
    window: "6m",
    estimated_messages: estimated,
    counts,
  };
}

type StubOptions = {
  /** Status rows served per GET, consumed one at a time (last repeats). */
  statuses: BackfillStatus[];
  preview?: BackfillPreview;
  /** The preview POST's own answer, for the cases that are about an estimate
   *  which never lands or never comes — where a `preview` row cannot say it. */
  previewRoute?: () => Response | Promise<Response>;
  /** The status the start POST flips the next GET to. */
  onStart?: BackfillStatus;
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubApi(options: StubOptions) {
  const calls: Request[] = [];
  const statuses = [...options.statuses];
  let started = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push(request);
      const url = new URL(request.url);
      const path = url.pathname;
      if (path.endsWith("/backfill/preview")) {
        return options.previewRoute
          ? options.previewRoute()
          : jsonResponse(options.preview ?? previewOf(400));
      }
      if (path.endsWith("/backfill") && request.method === "POST") {
        started = true;
        return jsonResponse(options.onStart ?? statusNone, 202);
      }
      if (path.endsWith("/backfill") && request.method === "DELETE") {
        // Cancelling advances the polled status to the next row (the
        // cancelled snapshot the test queued after the running one).
        if (statuses.length > 1) {
          statuses.shift();
        }
        return jsonResponse(statuses[0]);
      }
      if (path.endsWith("/backfill") && request.method === "GET") {
        if (started && options.onStart) {
          const row = statuses.length > 1 ? statuses.shift() : statuses[0];
          return jsonResponse(row ?? options.onStart);
        }
        return jsonResponse(statuses[0] ?? statusNone);
      }
      throw new Error(`unstubbed: ${request.method} ${path}`);
    }),
  );
  return calls;
}

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function requestsTo(calls: Request[], suffix: string, method: string) {
  return calls.filter(
    (r) => new URL(r.url).pathname.endsWith(suffix) && r.method === method,
  );
}

// The live figures count UP over a second of real time, and these cases are
// about the number rather than the climb. Reduced motion renders the end state
// at once, which is both the honest thing for the component to do and the only
// way to assert on a figure without a test whose cost is wall-clock.
function preferNoMotion() {
  vi.stubGlobal("matchMedia", ((query: string) => ({
    matches: query.includes("prefers-reduced-motion"),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia);
}

beforeEach(() => {
  preferNoMotion();
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the connect-time backfill payoff", () => {
  // The multi-year reach (ADR-0106) is only real if the picker offers it and
  // the chosen value reaches the server: the window set is stated in five
  // places, and this is the one a human touches.
  it("offers the whole window set, and previews the multi-year one against the server", async () => {
    const calls = stubApi({
      statuses: [statusNone],
      preview: previewOf(90210),
    });
    render(<BackfillPanel provider="gmail" />);
    await screen.findByRole("group");

    for (const label of [
      "3 months",
      "6 months",
      "12 months",
      "2 years",
      "5 years",
    ]) {
      expect(screen.getByRole("radio", { name: label })).toBeTruthy();
    }
    // 6 months stays the default: a multi-year import is offered, never
    // defaulted into — it spends the customer's own inference budget.
    expect(
      (screen.getByRole("radio", { name: "6 months" }) as HTMLInputElement)
        .checked,
    ).toBe(true);

    await userEvent.click(screen.getByRole("radio", { name: "5 years" }));
    await waitFor(async () => {
      const previews = requestsTo(calls, "/backfill/preview", "POST");
      const last = previews.at(-1);
      expect(last).toBeTruthy();
      expect(await last?.clone().json()).toMatchObject({ window: "60m" });
    });
  });

  // A capped count is a FLOOR, and shown as a count it is short by multiples on
  // exactly the mailboxes where the cap binds — five years of a routine
  // business mailbox crosses the limit. A reader has no way to tell the two
  // kinds of number apart, and this is the number they are consenting to.
  it("says 'at least' when the provider stopped counting", async () => {
    stubApi({
      statuses: [statusNone],
      preview: { ...previewOf(20000), estimate_is_floor: true },
    });
    render(<BackfillPanel provider="gmail" />);

    expect(
      await screen.findByText(/At least 20,000 messages in that period/),
    ).toBeTruthy();
    // And not as a plain count, which is the statement this replaces.
    expect(screen.queryByText(/^20,000 messages in that period/)).toBeNull();
  });

  it("auto-loads the scope estimate without a click, and does not spend until start", async () => {
    const calls = stubApi({ statuses: [statusNone], preview: previewOf(1234) });
    render(<BackfillPanel provider="gmail" />);

    // The scope appears with no user interaction, and the WINDOW leads it: the
    // period of their own mailbox is what the mailbox owner agrees to, and the count
    // describes that period.
    expect(await screen.findByText(/6 months of your mailbox/)).toBeTruthy();
    expect(screen.getByText(/1,234 messages in that period/)).toBeTruthy();
    expect(requestsTo(calls, "/backfill/preview", "POST").length).toBe(1);
    // But nothing has been imported: no start POST fired on its own.
    expect(requestsTo(calls, "/backfill", "POST").length).toBe(0);
  });

  // The estimate is what the reader consents to SPEND, so the unit printed on
  // it has to be the unit the server priced in. The contract pins v1 estimates
  // to USD minor units and leaves `currency` optional, which is why the
  // onboarding step of this same read falls back to USD — one field labelled
  // two ways by two screens means one of them is telling the reader the wrong
  // price.
  it("prices an unlabelled estimate in the contract's own unit, never in euro", async () => {
    stubApi({
      statuses: [statusNone],
      preview: { ...previewOf(400), estimated_cost_minor: 250 },
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText(/~US\$2\.50/)).toBeTruthy();
    expect(screen.queryByText(/EUR/)).toBeNull();
  });

  it("starts the import only on the explicit consent click", async () => {
    const calls = stubApi({
      statuses: [statusNone],
      preview: previewOf(400),
      onStart: countsStatus("running", { captured: 0 }),
    });
    render(<BackfillPanel provider="gmail" />);

    await screen.findByText(/400 messages in that period/);
    await userEvent.click(
      screen.getByRole("button", { name: /Start the import/ }),
    );

    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "POST").length).toBe(1),
    );
  });

  // The estimate describes the window; the window is what the reader picked
  // and what bounds the run. So the verb stands while the count is still
  // running — the card used to appear only once an estimate landed, which took
  // the start off screen for exactly the mailboxes slow enough to count.
  it("offers the start while the scope is still being counted", async () => {
    const calls = stubApi({
      statuses: [statusNone],
      // Never settles: the case is what the reader can do meanwhile.
      previewRoute: () => new Promise<Response>(() => {}),
      onStart: countsStatus("running", { captured: 0 }),
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText("Counting your mailbox…")).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: /Start the import/ }),
    );

    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "POST").length).toBe(1),
    );
  });

  // An estimator that refused left this card unrendered, so the only way to
  // import a mailbox whose scope could not be counted was to stop offering to.
  it("offers the start when the estimate was refused", async () => {
    const calls = stubApi({
      statuses: [statusNone],
      previewRoute: () =>
        jsonResponse(
          { code: "internal", detail: "No answer from Gmail." },
          502,
        ),
      onStart: countsStatus("running", { captured: 0 }),
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText(/No answer from Gmail\./)).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: /Start the import/ }),
    );

    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "POST").length).toBe(1),
    );
  });

  it("renders the three headline figures — captured, contacts, companies — from the run counts", async () => {
    stubApi({
      statuses: [
        countsStatus("running", {
          captured: 128,
          contacts_created: 47,
          companies_created: 12,
          messages_scanned: 150,
        }),
      ],
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText("128")).toBeTruthy();
    expect(screen.getByText("47")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("Emails captured")).toBeTruthy();
    expect(screen.getByText("Contacts")).toBeTruthy();
    // "to check", not created: the count is domains this run raised a company
    // question for, and a domain becomes a company only if its site says so.
    expect(screen.getByText("Companies to check")).toBeTruthy();
  });

  it("shows the celebratory done state when the run completes", async () => {
    stubApi({
      statuses: [
        countsStatus("done", {
          captured: 512,
          contacts_created: 90,
          companies_created: 20,
          messages_scanned: 600,
        }),
      ],
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText(/History import complete/i)).toBeTruthy();
    expect(screen.getByText("512")).toBeTruthy();
  });

  it("lets the user stop a running import and reflects the cancelled state", async () => {
    const calls = stubApi({
      statuses: [
        countsStatus("running", { captured: 20, messages_scanned: 40 }),
        countsStatus("cancelled", { captured: 20, messages_scanned: 40 }),
      ],
    });
    render(<BackfillPanel provider="gmail" />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Stop the import/ }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/backfill", "DELETE").length).toBe(1),
    );
    expect(await screen.findByText(/Stopped\./)).toBeTruthy();
  });

  // Stopping an import is a decision about the run, not about the mailbox: the
  // panel used to draw the stopped run forever, so a reader who pressed stop
  // could never import again — disconnecting and reconnecting did not help,
  // because the run rows belong to the connection that survives both.
  it("offers another import once a run has stopped, opening on the window that ran", async () => {
    const calls = stubApi({
      statuses: [
        {
          state: "cancelled",
          backfill_id: "018f3a1b-0000-7000-8000-0000000000b1",
          window: "12m",
          estimated_messages: 400,
          counts: { captured: 20, messages_scanned: 40 },
        },
      ],
      preview: previewOf(1234),
    });
    render(<BackfillPanel provider="gmail" />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start another import/ }),
    );
    // Opened on the window this mailbox already ran, because the server only
    // ever widens — a picker that opens on a refusal wastes the first press.
    expect(
      (
        (await screen.findByRole("radio", {
          name: "12 months",
        })) as HTMLInputElement
      ).checked,
    ).toBe(true);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start the import/ }),
    );
    await waitFor(async () => {
      const starts = requestsTo(calls, "/backfill", "POST");
      expect(starts.length).toBe(1);
      expect(await starts[0]?.clone().json()).toMatchObject({ window: "12m" });
    });
  });

  it("surfaces an honest error class without hiding the counts captured so far", async () => {
    stubApi({
      statuses: [countsStatus("error", { captured: 40, contacts_created: 9 })],
    });
    render(<BackfillPanel provider="gmail" />);

    expect(await screen.findByText("40")).toBeTruthy();
    expect(
      screen.getByText(/everything captured so far is kept/i),
    ).toBeTruthy();
  });
});

// The connections-card mount (connectors.tsx) seeds the panel from
// CaptureConnection.backfill, already embedded in GET /connectors — these
// exercise the honest branches that seed unlocks: a provider with no
// Backfiller, a run whose updated_at stopped moving, a null estimate, and a
// refused window narrowing. Real installFetchStub route-map stubs throughout.
describe("honest capability and staleness", () => {
  it("renders an unsupported source as a capability statement, not an error", async () => {
    installFetchStub({
      "POST /connectors/imap/backfill/preview": () =>
        jsonResponse({ code: "connector_unsupported" }, 422),
    });
    render(<BackfillPanel provider="imap" initial={{ state: "none" }} />);

    expect(await screen.findByText(/can't be backfilled/i)).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    // Not a retryable error state: no window picker offered for a provider
    // that structurally can't run this op.
    expect(screen.queryByRole("group")).toBeNull();
  });

  // The estimate is a floor: Gmail's exact count is capped at a page budget,
  // and a multi-year window reaches that cap far more often. A run that scans
  // past its own denominator has no percentage to show, and a full bar over a
  // still-running import would be the one number on this screen that lies.
  // An overrun used to mean one thing because the estimate always was a floor.
  // It is now two, and they read differently: a mailbox that grew past an EXACT
  // count leaves a run at its end, while a FLOOR that has been passed leaves
  // nobody knowing how much is left.
  it("drops the percentage once a run scans past a FLOOR it cannot see beyond", () => {
    render(
      <BackfillPanel
        provider="gmail"
        initial={{
          ...countsStatus("running", { captured: 900, messages_scanned: 900 }),
          estimated_messages: 500,
          estimate_is_floor: true,
        }}
      />,
    );

    expect(screen.queryByRole("progressbar")).toBeNull();
    // The absolute counts stay: scanned and captured, both past the bound.
    expect(screen.getAllByText(/900/).length).toBeGreaterThan(0);
  });

  it("keeps the bar full when an EXACT estimate was overrun by a growing mailbox", () => {
    render(
      <BackfillPanel
        provider="gmail"
        initial={{
          ...countsStatus("running", { captured: 900, messages_scanned: 900 }),
          estimated_messages: 500,
        }}
      />,
    );

    // Drawn rather than dropped: the provider counted every message in the
    // window, so what is left is the handful that arrived during the import —
    // and withholding the bar there would say the run's progress is unknowable
    // when it is nearly finished.
    const bar = screen.getByRole("progressbar");
    expect(bar.getAttribute("aria-valuenow")).toBe("100");
  });

  it("does not animate a running run whose updated_at is stale", () => {
    const staleUpdatedAt = new Date(Date.now() - 20 * 60_000).toISOString();
    render(
      <BackfillPanel
        provider="gmail"
        initial={{
          ...countsStatus("running", { captured: 40, messages_scanned: 40 }),
          updated_at: staleUpdatedAt,
        }}
      />,
    );

    expect(screen.getByText(/last updated/i)).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows absolute counts and no percentage when estimated_messages is null", () => {
    render(
      <BackfillPanel
        provider="gmail"
        initial={{
          state: "running",
          estimated_messages: null,
          counts: { captured: 12 },
        }}
      />,
    );

    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.queryByText(/%/)).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("explains a refused narrowing instead of failing generically", async () => {
    installFetchStub({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse(previewOf(400)),
      "POST /connectors/gmail/backfill": () =>
        jsonResponse({ code: "window_narrowing" }, 409),
    });
    render(<BackfillPanel provider="gmail" initial={{ state: "none" }} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start the import/ }),
    );

    expect(await screen.findByText(/only be widened/i)).toBeTruthy();
  });
});

// What the panel PUTS ON THE SCREEN for a failure, which is the half a reader
// ever sees. Keeping the same failure readable on the console belongs to the
// client's mutation sink and is pinned once against it (app/queryclient.test).
describe("a failure nobody wrote for a reader", () => {
  it("shows the shared line and never the transport wording", async () => {
    installFetchStub({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse(previewOf(400)),
      "POST /connectors/gmail/backfill": () => {
        throw new TypeError("ECONNREFUSED: connection refused");
      },
    });
    render(<BackfillPanel provider="gmail" initial={{ state: "none" }} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start the import/ }),
    );

    expect(
      await screen.findByText("The request failed. No cause reported."),
    ).toBeTruthy();
    // The wording nobody wrote for a user stays off the screen entirely.
    expect(screen.queryByText(/ECONNREFUSED/)).toBeNull();
  });

  it("shows the server's own cause when the server composed one", async () => {
    installFetchStub({
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse(previewOf(400)),
      "POST /connectors/gmail/backfill": () =>
        jsonResponse(
          { code: "quota_exhausted", detail: "This month's budget is spent." },
          429,
        ),
    });
    render(<BackfillPanel provider="gmail" initial={{ state: "none" }} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Start the import/ }),
    );

    expect(
      await screen.findByText("This month's budget is spent."),
    ).toBeTruthy();
  });
});
