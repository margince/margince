/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AiCallsCard, useLastCallAt } from "./aicalls";

const summary = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  occurred_at: "2026-07-20T10:00:00Z",
  kind: "completion",
  task: "capture_classify",
  task_display_name: "Message classification",
  tier: "cheap_cloud",
  provider: "gemini",
  model_id: "configured",
  served_model: "served",
  calls_attempted: 2,
  tokens_in: 100,
  tokens_out: 20,
  reasoning_tokens: 0,
  cached_tokens: 0,
  latency_ms: 900,
  cache_hit: false,
  degraded: true,
  error_sentinel: "provider_unavailable",
  has_payload: true,
  decision_attempted: false,
  outcome: "failed",
  estimated_cost_microusd: 4300,
};

// The trace is gated on `ai_diagnostics:read`, which is what `GET /ai/calls`
// asks for. A stub that never answers /me leaves the caller holding no grant,
// so the card says it is withheld instead of rendering rows.
//
// The card once asked `automation:update`, a write verb guarding a GET, which
// kept a management seat off a read the server would have served.
const OPERATOR: GrantSpec = { ai_diagnostics: ["read"] };

const RETRIED = {
  call: summary,
  attempts: [
    {
      attempt: 1,
      is_terminal: false,
      kind: "completion",
      attempt_reason: "",
      tokens_in: 100,
      tokens_out: 0,
      latency_ms: 400,
      occurred_at: summary.occurred_at,
    },
    {
      attempt: 2,
      is_terminal: true,
      kind: "completion",
      attempt_reason: "retry_on_5xx",
      tokens_in: 100,
      tokens_out: 20,
      latency_ms: 900,
      occurred_at: summary.occurred_at,
    },
  ],
};

// A decision model that answered below its floor, and the ladder rung that
// answered instead. The terminal call is the completion; the decision is an
// attempt before it, and the reason on the rung says why the walk went on.
const DECIDED_THEN_FELL_BACK = {
  call: {
    ...summary,
    degraded: false,
    error_sentinel: null,
    decision_attempted: true,
    outcome: "fallback",
  },
  attempts: [
    {
      attempt: 1,
      is_terminal: false,
      kind: "decision",
      tier: "decide",
      provider: "jev_compatible",
      model_id: "jev-classify",
      attempt_reason: "",
      decision_choice: "company",
      decision_confidence: 0.62,
      tokens_in: 40,
      tokens_out: 0,
      latency_ms: 600,
      occurred_at: summary.occurred_at,
    },
    {
      attempt: 2,
      is_terminal: true,
      kind: "completion",
      tier: "cheap_cloud",
      attempt_reason: "decision_below_floor",
      tokens_in: 100,
      tokens_out: 20,
      latency_ms: 900,
      occurred_at: summary.occurred_at,
    },
  ],
};

// A call the decision model answered itself: the terminal row is the decision.
const DECIDED = {
  call: {
    ...summary,
    kind: "decision",
    tier: "decide",
    provider: "jev_compatible",
    calls_attempted: 1,
    decision_attempted: true,
  },
  attempts: [DECIDED_THEN_FELL_BACK.attempts[0]],
};

function mount(
  captureEnabled = true,
  withPayload = true,
  allow: GrantSpec = OPERATOR,
  trace: {
    call: Record<string, unknown>;
    attempts: unknown[];
  } = RETRIED,
  others: Record<string, unknown>[] = [],
  listed: boolean | ((query: URLSearchParams) => boolean) = true,
) {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test",
      );
      const path = url.pathname;
      seen.push(path);
      const answered =
        typeof listed === "function" ? listed(url.searchParams) : listed;
      if (path.endsWith("/v1/me")) {
        return new Response(JSON.stringify(meFixture({ allow })), {
          headers: { "Content-Type": "application/json" },
        });
      }
      const other = others.find((row) => path.endsWith(String(row.id)));
      const body =
        path.endsWith(summary.id) || other
          ? {
              ...(other ?? trace.call),
              served_identity_source: "response",
              context_scopes: [],
              context_fingerprint: "",
              attempts: trace.attempts,
              payload_captured: withPayload,
              payload: withPayload
                ? { request: { system: "safe", messages: [] }, response: "ok" }
                : null,
            }
          : {
              data: answered ? [trace.call, ...others] : [],
              page: { has_more: false },
              payload_capture_enabled: captureEnabled,
              tasks: [summary.task],
              task_options: [
                { task: summary.task, display_name: summary.task_display_name },
              ],
              models: [summary.served_model],
            };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AiCallsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { seen };
}

// The hook's four answers, read through a component that renders nothing else.
// A probe rather than a second card: what this file pins is the STATE, and the
// surface that acts on it is under test beside the card that draws it.
function LastCallProbe() {
  const last = useLastCallAt();
  return (
    <output>
      {last.state === "at" ? new Date(last.epochMs).toISOString() : last.state}
    </output>
  );
}

// The same server the card meets, with the trace read answered per case. The
// probe is mounted alone so nothing else on screen can satisfy a query for it.
function mountProbe(
  trace: () => Promise<Response>,
  allow: GrantSpec = OPERATOR,
) {
  const seen: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test",
      ).pathname;
      seen.push(path);
      return path.endsWith("/v1/me")
        ? new Response(JSON.stringify(meFixture({ allow })), {
            headers: { "Content-Type": "application/json" },
          })
        : trace();
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LastCallProbe />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { seen };
}

function tracePage(rows: unknown[]) {
  return async () =>
    new Response(
      JSON.stringify({
        data: rows,
        page: { has_more: false },
        tasks: [],
        task_options: [],
        models: [],
        payload_capture_enabled: false,
      }),
      { headers: { "Content-Type": "application/json" } },
    );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Four silences, and only one of them is a claim about the INSTALLATION.
//
// The withheld case and the answered ones are asserted from one fixture pair on
// purpose: "withheld" is also what the probe reads while /me is still in
// flight, so it proves nothing until the same wiring one grant apart reaches an
// instant.
it("says the trace is withheld rather than that nothing was ever called", async () => {
  const { seen } = mountProbe(tracePage([summary]), {
    automation: ["read", "update"],
  });

  expect(await screen.findByText("withheld")).toBeTruthy();
  // And the denial is already known, so the read never fires.
  expect(seen.some((path) => path.includes("/ai/calls"))).toBe(false);
});

it("answers with the newest call's instant once the trace has landed", async () => {
  mountProbe(tracePage([summary]));

  // The instant the newest row carries, parsed: not a zero and not the row
  // below it.
  expect(
    await screen.findByText(new Date(summary.occurred_at).toISOString()),
  ).toBeTruthy();
});

it("says nothing is read yet while the trace is still arriving", async () => {
  // A request that never settles IS the in-flight state, with no clock and
  // nothing to wait out.
  const { seen } = mountProbe(() => new Promise<Response>(() => {}));

  await waitFor(() =>
    expect(seen.some((path) => path.includes("/ai/calls"))).toBe(true),
  );
  expect(screen.getByText("unread")).toBeTruthy();
});

// A failed read is its own answer, for the reason "never" is: only one of the
// two resolves by waiting, and a caller that could not tell them apart would
// draw a reading that goes quiet on a broken read.
it("says the trace read failed rather than that it is still arriving", async () => {
  mountProbe(async () => new Response("", { status: 500 }));

  await waitFor(() => expect(screen.getByText("failed")).toBeTruthy());
  expect(screen.queryByText("unread")).toBeNull();
});

it("says never called only when the trace answered and held no row", async () => {
  mountProbe(tracePage([]));

  expect(await screen.findByText("never")).toBeTruthy();
});

// The drawer is portalled to the body and named by the call it opens.
async function openCall(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", {
      name: /^Open the Message classification call/,
    }),
  );
  return screen.findByRole("dialog");
}

it("reads each call in five columns: when, feature, model, outcome and cost", async () => {
  mount();
  const row = await screen.findByTestId(`call-${summary.id}`);
  expect(within(row).getByText("Message classification")).toBeTruthy();
  expect(row.textContent).toContain("Failed");
  expect(row.textContent).toContain("$0.0043");
  // Tokens and latency are the drawer's, so the row fits the settings column.
  expect(row.textContent).not.toContain("100 / 20");
  expect(row.textContent).not.toContain("900 ms");
  // A code this screen has no words for keeps its key, under its badge.
  expect(within(row).getByText("provider_unavailable")).toBeTruthy();
});

it("opens a call in a drawer and leaves the table exactly as it was", async () => {
  const user = userEvent.setup();
  mount();
  const table = await screen.findByRole("table");
  const rowsBefore = table.querySelectorAll("tr").length;

  const drawer = await openCall(user);
  // Nothing is inserted under the row: the detail lives beside the table.
  expect(table.querySelectorAll("tr").length).toBe(rowsBefore);
  expect(within(drawer).getByText("Message classification")).toBeTruthy();
  expect(await within(drawer).findByText(/retry.on.5xx/i)).toBeTruthy();
  expect(within(drawer).getByText("Request payload")).toBeTruthy();
  expect(
    within(drawer).getByText("Export certification scenario"),
  ).toBeTruthy();

  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("puts focus on the drawer itself when a call opens, not on a stepping arrow", async () => {
  const user = userEvent.setup();
  mount();
  const drawer = await openCall(user);
  await waitFor(() => expect(document.activeElement).toBe(drawer));
});

it("copies a payload and says Copied on that block alone", async () => {
  const user = userEvent.setup();
  const clipboard = stubClipboard("accepts");
  mount();
  const drawer = await openCall(user);
  await user.click(
    await within(drawer).findByRole("button", {
      name: "Copy Response payload",
    }),
  );
  await waitFor(() => expect(clipboard.written).toEqual(["ok"]));
  expect(within(drawer).getByRole("button", { name: "Copied" })).toBeTruthy();
  expect(
    within(drawer).getByRole("button", { name: "Copy Request payload" }),
  ).toBeTruthy();
});

it("steps to the next call from inside the drawer", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, RETRIED, [
    {
      ...summary,
      id: "second-call",
      task: "enrich",
      task_display_name: "Enrichment",
      error_sentinel: null,
    },
  ]);
  const drawer = await openCall(user);
  expect(
    within(drawer).getByRole("button", { name: "Previous call" }),
  ).toHaveProperty("disabled", true);
  await user.click(within(drawer).getByRole("button", { name: "Next call" }));
  expect(
    await within(drawer).findByRole("heading", {
      name: en["aiTask.name.enrich"],
    }),
  ).toBeTruthy();
  expect(
    within(drawer).getByRole("button", { name: "Next call" }),
  ).toHaveProperty("disabled", true);
});

// The page already names the log, so the trace carries no heading of its own.
// The task dial stands in a bar above it, and the table keeps its name.
it("puts the task filter in a bar above the trace, and repeats no page heading", async () => {
  mount();
  const filter = await screen.findByRole("combobox", { name: "Feature" });
  const bar = filter.closest("fieldset");
  expect(bar).not.toBeNull();
  const trace = await screen.findByRole("table", { name: "Recent calls" });
  expect(bar?.contains(trace)).toBe(false);
  expect(screen.queryByRole("heading")).toBeNull();
});

// A task row on the models page links here with the task on the address, so the
// filter is read from it and written back to it.
it("narrows the trace to the feature the address names, and writes a new pick back", async () => {
  const user = userEvent.setup();
  globalThis.location.hash = "#/settings/model-calls?task=account_scan";
  mount();
  const filter = await screen.findByRole("combobox", { name: "Feature" });
  // No call for it yet, so the server's set lacks it; the select still says it.
  expect(filter.textContent).toContain("Reading what an account needs");
  const asked = vi
    .mocked(fetch)
    .mock.calls.map(([input]) =>
      input instanceof Request ? input.url : String(input),
    );
  expect(asked.some((url) => url.includes("task=account_scan"))).toBe(true);

  await user.click(filter);
  await user.click(
    await screen.findByRole("option", { name: "Message classification" }),
  );
  expect(globalThis.location.hash).toBe(
    `#/settings/model-calls?task=${summary.task}`,
  );
  globalThis.location.hash = "";
});

function askedQueries(): URLSearchParams[] {
  return vi
    .mocked(fetch)
    .mock.calls.map(
      ([input]) =>
        new URL(
          input instanceof Request ? input.url : String(input),
          "https://test",
        ),
    )
    .filter((url) => url.pathname.endsWith("/ai/calls"))
    .map((url) => url.searchParams);
}

// Every dial is a question the server answers over the whole trace. A pick is
// written to the address and asked of the server. An empty answer offers the
// way back rather than claiming no such call exists.
it("asks the server for the outcome and search picked, and clears both", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, RETRIED, [], (query) => !query.has("q"));
  await user.click(await screen.findByRole("combobox", { name: "Outcome" }));
  await user.click(await screen.findByRole("option", { name: "Failed" }));
  expect(globalThis.location.hash).toContain("outcome=failed");
  await waitFor(() =>
    expect(askedQueries().some((q) => q.get("outcome") === "failed")).toBe(
      true,
    ),
  );

  await user.type(
    screen.getByRole("searchbox", { name: "Search calls" }),
    "quota",
  );
  expect(await screen.findByText("No calls match these filters.")).toBeTruthy();
  // The search is asked once it settles, never once per keystroke.
  const searched = askedQueries().filter((q) => q.has("q"));
  expect(searched.map((q) => q.get("q"))).toEqual(["quota"]);

  // The bar and the empty answer both offer the way back; they do one thing.
  await user.click(
    within(screen.getByRole("group")).getByRole("button", {
      name: "Clear filters",
    }),
  );
  expect(await screen.findByTestId(`call-${summary.id}`)).toBeTruthy();
  expect(globalThis.location.hash).not.toContain("outcome");
  expect(
    (
      screen.getByRole("searchbox", {
        name: "Search calls",
      }) as HTMLInputElement
    ).value,
  ).toBe("");
  globalThis.location.hash = "";
});

it("says a call the price sheet cannot price is not priced, never free", async () => {
  mount(true, true, OPERATOR, {
    call: { ...summary, estimated_cost_microusd: undefined },
    attempts: RETRIED.attempts,
  });
  const row = await screen.findByTestId(`call-${summary.id}`);
  expect(row.textContent).toContain("Not priced");
  expect(row.textContent).not.toContain("$0.00");
});

it("distinguishes capture disabled from a call without payload", async () => {
  const user = userEvent.setup();
  mount(false, false);
  let drawer = await openCall(user);
  expect(
    await within(drawer).findByText(/Payload capture is off/),
  ).toBeTruthy();
  cleanup();
  mount(true, false);
  drawer = await openCall(user);
  expect(
    await within(drawer).findByText("No payload captured for this call."),
  ).toBeTruthy();
});

it("withholds the trace from a principal without the diagnostics read, and asks the server for nothing", async () => {
  // Withheld, not absent: an absent trace claims the installation made no model
  // calls. The list read never fires, because the denial is already known.
  const { seen } = mount(true, true, { automation: ["read", "update"] });

  expect(
    await screen.findByText(
      /only an administrator or operations user can read the call trace/i,
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("searchbox")).toBeNull();
  expect(seen.some((path) => path.includes("/ai/calls"))).toBe(false);
});

it("names the decision lane in words, never as the rung key", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, DECIDED);
  const drawer = await openCall(user);
  expect(within(drawer).getAllByText("Decision model").length).toBeGreaterThan(
    0,
  );
  expect(screen.queryByText(/\bdecide\b/)).toBeNull();
});

// The terminal row of a fallback is the completion. The drawer reads the
// logical call's flag rather than its own kind. A decision model that was asked
// and overruled is then marked in the drawer head.
it("marks a call the decision model fell back from, and never as a retry", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, DECIDED_THEN_FELL_BACK);
  const row = await screen.findByTestId(`call-${summary.id}`);
  // The fall back is a second model asked, not a retry of the first.
  expect(row.textContent).not.toContain("Retried");
  const drawer = await openCall(user);
  expect(
    within(drawer).getAllByText(en["aicalls.badge.decision"]).length,
  ).toBeGreaterThan(0);
});

it("draws no outcome badge on a first-try answer, and no decision mark", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, {
    call: {
      ...summary,
      calls_attempted: 1,
      degraded: false,
      error_sentinel: null,
      outcome: "answered",
    },
    attempts: [RETRIED.attempts[1]],
  });
  const row = await screen.findByTestId(`call-${summary.id}`);
  expect(row.querySelector(".badge")).toBeNull();
  const drawer = await openCall(user);
  expect(within(drawer).queryByText("Decision model")).toBeNull();
});

it("marks a call that answered after a failed attempt as a warning", async () => {
  mount(true, true, OPERATOR, {
    call: { ...summary, degraded: false, error_sentinel: null },
    attempts: RETRIED.attempts,
  });
  const row = await screen.findByTestId(`call-${summary.id}`);
  expect(
    within(row).getByText("Retried").closest(".badge")?.className,
  ).toContain("badge-warning");
});

it("names a failure code on the row and in the attempt that hit it", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, {
    call: { ...summary, degraded: false, error_sentinel: "output_rejected" },
    attempts: [
      { ...RETRIED.attempts[0] },
      { ...RETRIED.attempts[1], error_sentinel: "output_rejected" },
    ],
  });
  const row = await screen.findByTestId(`call-${summary.id}`);
  expect(within(row).getByText("Answer rejected")).toBeTruthy();
  expect(within(row).queryByText("output_rejected")).toBeNull();
  const drawer = await openCall(user);
  const second = (await within(drawer).findByText("#2")).closest("li");
  expect(second?.textContent).toContain("Answer rejected");
});

it("narrows the trace to where a figure's calls ended, and lets the reader clear it", async () => {
  const user = userEvent.setup();
  globalThis.location.hash =
    "#/settings/model-calls?provider=openai_compatible&served_provider=Cerebras";
  mount();
  // Named in the filter bar's caption, beside the count of what it let through.
  expect(
    await screen.findByText(
      /^Showing calls that ended on Provider: OpenAI-compatible · Served by: Cerebras\./,
    ),
  ).toBeTruthy();
  expect(
    askedQueries().some(
      (q) =>
        q.get("provider") === "openai_compatible" &&
        q.get("served_provider") === "Cerebras",
    ),
  ).toBe(true);

  await user.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(globalThis.location.hash).toBe("#/settings/model-calls");
  globalThis.location.hash = "";
});

it("shows the request settings the attempts were sent with, and the host that served one", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, {
    call: {
      ...summary,
      config: {
        task_contract_hash: "c",
        routing_config_hash: "r",
        prompt_version: "p",
        provider_params: { provider: { sort: "latency" }, deadline_ms: 30000 },
      },
    },
    attempts: [
      { ...RETRIED.attempts[0] },
      {
        ...RETRIED.attempts[1],
        attempt_reason: "timeout",
        served_provider: "Cerebras",
      },
    ],
  });
  const drawer = await openCall(user);
  expect(await within(drawer).findByText("Request settings sent")).toBeTruthy();
  expect(within(drawer).getByText(/"deadline_ms": 30000/)).toBeTruthy();
  expect(within(drawer).getByText(/served by Cerebras/)).toBeTruthy();
  expect(
    within(drawer).getByText(/The attempt before ran out of time/),
  ).toBeTruthy();
});

it("says why the ladder answered after the decision model, and where it went", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, DECIDED_THEN_FELL_BACK);
  const drawer = await openCall(user);

  // The fallback rung reads its tier, then the reason in words rather than the
  // wire code, and no number: the floor is not stored on the call.
  expect(
    await within(drawer).findByText(
      /Everyday cloud · Decision model below its confidence floor/,
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/decision_below_floor/)).toBeNull();
  const first = within(drawer).getByText("#1").closest("li");
  expect(first?.textContent).toContain("Decision model");
  expect(first?.textContent).toContain("jev-classify · Jev-compatible");
});

// The answer that did not stand is the one a floor is tuned from, so the
// decision attempt says what it answered even when the ladder answered after.
it("says what the decision model answered, and at what confidence", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, DECIDED_THEN_FELL_BACK);
  const drawer = await openCall(user);

  const first = (await within(drawer).findByText("#1")).closest("li");
  expect(first?.textContent).toContain("answered company at 0.62");
  const second = within(drawer).getByText("#2").closest("li");
  expect(second?.textContent).not.toContain("answered");
});

// The rung a decision model fell back to is the second model asked, not a
// second try of the first. The attempt count is the ladder's own.
it("counts the ladder's attempts without the decision model before them", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, {
    ...DECIDED_THEN_FELL_BACK,
    call: { ...DECIDED_THEN_FELL_BACK.call, calls_attempted: 3 },
  });
  const drawer = await openCall(user);
  const attempts = within(drawer).getByText("Attempts", { selector: "dt" });
  expect(attempts.nextElementSibling?.textContent).toBe("2");
});

// An ordinary first attempt and a decision that stood have no reason to run,
// so the line names its binding and its latency and nothing in between.
it("leaves the reason out of an attempt that had none", async () => {
  const user = userEvent.setup();
  mount(true, true, OPERATOR, DECIDED_THEN_FELL_BACK);
  const drawer = await openCall(user);

  const first = (await within(drawer).findByText("#1")).closest("li");
  expect(first?.textContent).toContain(
    "jev-classify · Jev-compatible · 600 ms",
  );
  expect(first?.textContent).not.toContain("\u2014");
});

// Under a feature filter an empty trace is a fact about that feature. It does
// not claim the installation never called a model, and offers the way back.
it("says a filtered feature has no calls, rather than that nothing was ever called", async () => {
  const user = userEvent.setup();
  globalThis.location.hash = "#/settings/model-calls?task=account_scan";
  mount(true, true, OPERATOR, RETRIED, [], false);
  expect(
    await screen.findByText("No calls recorded for this feature yet."),
  ).toBeTruthy();
  expect(screen.queryByText("No AI calls recorded yet.")).toBeNull();
  // The bar and the empty answer both offer the way back; they do one thing.
  await user.click(
    within(screen.getByRole("group")).getByRole("button", {
      name: "Clear filters",
    }),
  );
  expect(globalThis.location.hash).not.toContain("account_scan");
  globalThis.location.hash = "";
});

// The window is kept on the address by name and cut into an instant when the
// request runs. The server then narrows the whole trace, not the loaded pages.
it("asks the server for calls inside the time range picked", async () => {
  const user = userEvent.setup();
  mount();
  await user.click(await screen.findByRole("combobox", { name: "Time range" }));
  await user.click(await screen.findByRole("option", { name: "Last hour" }));
  expect(globalThis.location.hash).toContain("window=hour");
  await waitFor(() =>
    expect(askedQueries().some((q) => q.has("from"))).toBe(true),
  );
  const from = Date.parse(
    askedQueries()
      .find((q) => q.has("from"))
      ?.get("from") ?? "",
  );
  expect(Date.now() - from).toBeGreaterThanOrEqual(3_600_000);
  expect(Date.now() - from).toBeLessThan(3_600_000 + 60_000);
  globalThis.location.hash = "";
});

it("says in the drawer when the AI budget demoted the call", async () => {
  const user = userEvent.setup();
  mount();
  const drawer = await openCall(user);
  expect(within(drawer).getByText("Budget degraded")).toBeTruthy();
});
