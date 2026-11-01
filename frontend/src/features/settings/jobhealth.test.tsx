/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { tableRowCells } from "../../testing/tablerow.testkit";
import { JobHealthCard } from "./jobhealth";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// Every request this card could make, recorded, so a test can assert what
// actually went to the server — the absence of a call is the whole point of the
// non-admin case below (privacy.test.tsx's harness shape, copied per file per
// house convention).
type Sent = { key: string };

function stubRoutes(overrides: Record<string, () => Response> = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push({ key });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /admin/job-health") return jsonResponse(HEALTH);
      // `GET /admin/job-health` gates on `job_health:read`, so the default
      // principal here holds that grant. A test asserting the refusal overrides
      // this with a principal who does not.
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

const HEALTH = {
  generated_at: "2026-08-13T09:30:00Z",
  dead_window_hours: 24,
  kinds: [
    {
      kind: "capture_classify",
      queue: "default",
      fleet_wide: false,
      waiting: 12,
      running: 1,
      retrying: 2,
      dead: 0,
      discarded: 0,
      cancelled: 0,
      dead_recent: 0,
      oldest_waiting_age_seconds: 4_500,
      condition: "technical",
      condition_code: "queue_overdue",
      expectation_seconds: 60,
    },
    {
      kind: "retention_sweep_dispatch",
      queue: "periodic",
      fleet_wide: true,
      waiting: 0,
      running: 0,
      retrying: 0,
      dead: 0,
      discarded: 0,
      cancelled: 0,
      dead_recent: 0,
      oldest_waiting_age_seconds: null,
    },
  ],
  recent_failures: [
    {
      kind: "capture_classify",
      state: "retryable",
      attempt: 2,
      max_attempts: 5,
      failed_at: "2026-08-13T09:20:00Z",
      job_id: 4711,
      first_failed_at: "2026-08-13T09:08:00Z",
      failure_class: "provider_unavailable",
      remedy:
        "check the provider status page; the retry ladder rides out a brief outage",
      reason: "the model provider refused the request",
    },
  ],
};

// A failure whose stored text the job layer could not vet: it keeps the fixed
// substitute sentence and asserts NO class, so neither a class nor a remedy is
// on the wire. The endpoint nulls the pair together, which is why the tests
// below drop both at once rather than one at a time.
const UNCLASSIFIED = {
  kind: "capture_classify",
  state: "retryable",
  attempt: 2,
  max_attempts: 5,
  failed_at: "2026-08-13T09:20:00Z",
  job_id: 4711,
  first_failed_at: null,
  failure_class: null,
  remedy: null,
  reason: "the job failed for a reason it could not classify",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("JobHealthCard", () => {
  it("reports every state of every kind, and which rows carry no company", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    // Every count, zeros included: "0 dead" is the reassurance an operator
    // opened this card for.
    const kind = await tableRowCells("This company", "capture_classify");
    expect(kind.Waiting).toHaveTextContent("12");
    expect(kind.Running).toHaveTextContent("1");
    expect(kind.Retrying).toHaveTextContent("2");
    expect(kind.Discarded).toHaveTextContent("0");
    expect(kind.Queue).toHaveTextContent("default");
    expect(kind.State).toHaveTextContent("Needs technical attention");
    expect(kind["Declared wait limit"]).toHaveTextContent("1 minute");
    // The kind is River's identifier, verbatim, in both the count row and the
    // failure below it, which is how an operator ties the two together.
    expect(screen.getAllByText("capture_classify").length).toBe(2);
    const dispatcher = await tableRowCells(
      "Shared background work",
      "retention_sweep_dispatch",
    );
    expect(dispatcher.Discarded).toHaveTextContent("0");
    expect(
      screen.getByText(/counts are not limited to this company/i),
    ).toBeInTheDocument();
    // Each reading is named, so fleet work never reads as this company's.
    expect(
      screen.getByRole("heading", { name: "This company" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Recent failures" }),
    ).toBeInTheDocument();
    // 4,500 seconds is one hour in its largest whole unit, never "0 hr".
    expect(kind["Oldest wait"]).toHaveTextContent(/^1 hour$/);
  });

  it("names a kind it knows over its key, and shows an unknown kind's key once", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    const counted = await tableRowCells("This company", "capture_classify");
    expect(within(counted.Job).getByText("Mail sorting")).toBeInTheDocument();
    expect(within(counted.Job).getByText("capture_classify").tagName).toBe(
      "CODE",
    );
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(within(failure.Job).getByText("Mail sorting")).toBeInTheDocument();
    const unnamed = await tableRowCells(
      "Shared background work",
      "retention_sweep_dispatch",
    );
    expect(unnamed.Job).toHaveTextContent(/^retention_sweep_dispatch$/);
  });

  it("states nothing is runnable rather than claiming a wait of zero", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    const dispatcher = await tableRowCells(
      "Shared background work",
      "retention_sweep_dispatch",
    );
    // A null age means nothing of the kind is queued, which is not a wait of zero.
    expect(dispatcher.Queue).toHaveTextContent("periodic");
    expect(dispatcher["Oldest wait"]).toBeEmptyDOMElement();
  });

  it("surfaces the vetted failure text with the attempt it died on", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    expect(
      await screen.findByText("the model provider refused the request"),
    ).toBeInTheDocument();
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(failure.Attempt).toHaveTextContent("2 of 5");
    expect(screen.getByText(/come from the job layer/i)).toBeInTheDocument();
  });

  it("names the class, the remedy, the row and how long it has been failing", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    // The class is the token an alert is keyed on and the log is grepped by, so
    // it is rendered verbatim — the same treatment as the kind, underscores
    // kept — and never as a second status pill beside the state badge.
    const shownClass = await screen.findByText("provider_unavailable");
    expect(screen.queryByText(/provider unavailable/i)).not.toBeInTheDocument();
    expect(shownClass).not.toHaveClass("badge");
    // What to do about it, which is the half a failure list is useless without.
    expect(
      screen.getByText(/check the provider status page/),
    ).toBeInTheDocument();
    // River's own row id, ungrouped: "4,711" would not find the log line that
    // carries job_id=4711.
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(failure["Job ID"]).toHaveTextContent(/^4711$/);
    // Failing SINCE, which the attempt counter cannot say: rung 2 of 5 is the
    // same reading whether the first failure was a minute or a day ago.
    //
    // The formatted VALUE, not just the label. Matching the label alone passes
    // when the renderer interpolates nothing, which is the same dangling
    // "failing since" this screen is careful everywhere else not to draw. The
    // year is the stable part across locales; the fixture's is 2026-08-13.
    expect(failure["Failed at"].textContent).toMatch(/failing since .*2026/);
  });

  it("asserts no class, no remedy and no span for a failure nobody could vet", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({ ...HEALTH, recent_failures: [UNCLASSIFIED] }),
    });
    render(<JobHealthCard />);
    // The vetted substitute still reports what little is known.
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(
      screen.getByText("the job failed for a reason it could not classify"),
    ).toBeInTheDocument();
    // Absent stays absent. An unclassified failure has no class token and no
    // remedy line, and first_failed_at is null here — which the endpoint sends
    // only for a job that recorded NO attempt error at all. A single recorded
    // error does produce a first_failed_at, so null is absence and not "one".
    expect(screen.queryByText("provider_unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText(/What to do/)).not.toBeInTheDocument();
    expect(failure["Failed at"].textContent).not.toMatch(/failing since/);
    expect(failure.Reason.querySelectorAll("code")).toHaveLength(0);
  });

  it("reports the class without a span when only the first failure is missing", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          ...HEALTH,
          recent_failures: [
            { ...HEALTH.recent_failures[0], first_failed_at: null },
          ],
        }),
    });
    render(<JobHealthCard />);
    // The class and remedy do not depend on the span: a job cancelled before it
    // ran records no attempt error, and it is still a classified failure.
    expect(await screen.findByText("provider_unavailable")).toBeInTheDocument();
    expect(
      screen.getByText(/check the provider status page/),
    ).toBeInTheDocument();
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(failure["Failed at"].textContent).not.toMatch(/failing since/);
  });

  it("omits the job id rather than inventing one when the row carries none", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          ...HEALTH,
          recent_failures: [
            { ...HEALTH.recent_failures[0], job_id: undefined },
          ],
        }),
    });
    render(<JobHealthCard />);
    // job_id is optional on the wire, so a report from a build that predates it
    // must lose the label with the value rather than print a bare "job".
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(failure["Job ID"]).toBeEmptyDOMElement();
    // Dropping the id costs the id and nothing standing next to it.
    expect(failure["Failed at"].textContent).toMatch(/failing since .*2026/);
  });

  it("gives a dead job the danger treatment and says what it means", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          ...HEALTH,
          kinds: [
            {
              ...HEALTH.kinds[0],
              dead: 3,
              discarded: 3,
              dead_recent: 3,
              retrying: 0,
            },
          ],
          recent_failures: [
            { ...HEALTH.recent_failures[0], state: "discarded", attempt: 5 },
          ],
        }),
    });
    render(<JobHealthCard />);
    await tableRowCells("This company", "capture_classify");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    // And the count itself carries the tone on the row it belongs to.
    const kind = await tableRowCells("This company", "capture_classify");
    expect(within(kind.Discarded).getByText("3").closest(".badge")).toHaveClass(
      "badge-danger",
    );
    const failure = await tableRowCells("Recent failures", "capture_classify");
    expect(
      within(failure.State).getByText("Discarded").closest(".badge"),
    ).toHaveClass("badge-danger");
  });

  // The case the window exists for: a settled outage. The rows are still there
  // — River keeps them a week — and the banner must go quiet while the figure
  // does not, or a finished outage goes on asking for a hand until they retire.
  it("goes quiet once the dead work leaves the window, and still reports it", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          ...HEALTH,
          kinds: [
            {
              ...HEALTH.kinds[0],
              dead: 531,
              discarded: 531,
              dead_recent: 0,
              retrying: 0,
            },
          ],
        }),
    });
    render(<JobHealthCard />);
    const kind = await tableRowCells("This company", "capture_classify");
    expect(kind.Discarded).toHaveTextContent("531");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps intentional cancellations separate from discarded work", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          ...HEALTH,
          kinds: [
            {
              ...HEALTH.kinds[0],
              discarded: 0,
              cancelled: 4,
              dead: 4,
              dead_recent: 4,
            },
          ],
        }),
    });
    render(<JobHealthCard />);
    const kind = await tableRowCells("This company", "capture_classify");
    expect(kind.Canceled).toHaveTextContent("4");
    expect(kind.Canceled.querySelector(".badge-danger")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps a healthy report free of the dead-work alert", async () => {
    stubRoutes();
    render(<JobHealthCard />);
    await tableRowCells("This company", "capture_classify");
    // The fixture's dead counts are all zero, so nothing may interrupt. Without
    // this, an unconditional callout would pass the test above unnoticed.
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("withholds the report from a reader without the grant instead of asking for it", async () => {
    const sent = stubRoutes({
      "GET /me": () => jsonResponse(meFixture({ roles: ["ops"] })),
    });
    render(<JobHealthCard />);
    await screen.findByText(
      /background job health covers the whole installation/i,
    );
    expect(screen.queryByText("capture_classify")).not.toBeInTheDocument();
    // And it never issued the call the server would only refuse. An ops seat
    // reaching this page for its other sections must not generate a 403.
    expect(sent.some((entry) => entry.key === "GET /admin/job-health")).toBe(
      false,
    );
  });

  it("offers a retry on failure, and re-reads when it is taken", async () => {
    const sent = stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse(
          {
            title: "Internal Server Error",
            detail: "the job store could not be read",
            status: 500,
            code: "internal",
          },
          500,
        ),
    });
    render(<JobHealthCard />);
    expect(
      await screen.findByText("the job store could not be read"),
    ).toBeInTheDocument();
    const before = sent.filter(
      (entry) => entry.key === "GET /admin/job-health",
    ).length;
    await userEvent.click(screen.getByRole("button", { name: /retry/i }));
    await waitFor(() =>
      expect(
        sent.filter((entry) => entry.key === "GET /admin/job-health").length,
      ).toBeGreaterThan(before),
    );
  });

  it("says the queue is idle rather than showing an empty card", async () => {
    stubRoutes({
      "GET /admin/job-health": () =>
        jsonResponse({
          generated_at: "2026-08-13T09:30:00Z",
          kinds: [],
          recent_failures: [],
        }),
    });
    render(<JobHealthCard />);
    expect(
      await screen.findByText(/background queue is empty/i),
    ).toBeInTheDocument();
    // In its own words, INSTEAD of the readings — not three named rows each
    // saying it has nothing. That the background system is idle is one finding,
    // and a list of empty rows reports it three times as three.
    expect(screen.queryByText("This company")).not.toBeInTheDocument();
    expect(screen.queryByText("Recent failures")).not.toBeInTheDocument();
  });
});
