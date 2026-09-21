/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiCallsCard, useLastCallAt } from "./aicalls";

const summary = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  occurred_at: "2026-07-20T10:00:00Z",
  task: "capture_classify",
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
};

// The trace is gated on `ai_diagnostics:read`, which is what `GET /ai/calls`
// asks for — so a stub that never answers /me leaves the caller holding no
// grant and the card correctly says it is withheld instead of rendering rows.
//
// The card asked `automation:update` before the runtime's spend got an object of
// its own: a write verb guarding a GET, which kept a management seat off a read
// the server would have served.
const OPERATOR: GrantSpec = { ai_diagnostics: ["read"] };

function mount(
  captureEnabled = true,
  withPayload = true,
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
      if (path.endsWith("/v1/me")) {
        return new Response(JSON.stringify(meFixture({ allow })), {
          headers: { "Content-Type": "application/json" },
        });
      }
      const body = path.endsWith(summary.id)
        ? {
            ...summary,
            served_identity_source: "response",
            context_scopes: [],
            context_fingerprint: "",
            attempts: [
              {
                attempt: 1,
                is_terminal: false,
                attempt_reason: "",
                tokens_in: 100,
                tokens_out: 0,
                latency_ms: 400,
                occurred_at: summary.occurred_at,
              },
              {
                attempt: 2,
                is_terminal: true,
                attempt_reason: "retry_on_5xx",
                tokens_in: 100,
                tokens_out: 20,
                latency_ms: 900,
                occurred_at: summary.occurred_at,
              },
            ],
            payload_captured: withPayload,
            payload: withPayload
              ? { request: { system: "safe", messages: [] }, response: "ok" }
              : null,
          }
        : {
            data: [summary],
            page: { has_more: false },
            payload_capture_enabled: captureEnabled,
            tasks: [summary.task],
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

  // The instant the newest row carries, parsed — not a zero and not the row
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

// A failed read knows no more than an unfinished one, and neither is evidence
// about the installation. The card that OWNS the trace draws the failure and
// its retry; this reading simply stays silent.
it("says nothing is read yet when the trace read failed", async () => {
  mountProbe(async () => new Response("", { status: 500 }));

  await waitFor(() => expect(screen.getByText("unread")).toBeTruthy());
});

it("says never called only when the trace answered and held no row", async () => {
  mountProbe(tracePage([]));

  expect(await screen.findByText("never")).toBeTruthy();
});

it("renders call badges and expands the attempt and payload detail", async () => {
  mount();
  expect(await screen.findByText("provider_unavailable")).toBeTruthy();
  expect(screen.getByText("Retry ×2")).toBeTruthy();
  // One element, not the second of two: the task name used to appear in the
  // filter's option list as well as in the row, and the row is what expands.
  // The disclosure is a real button now, not the row: a `<tr onClick>`
  // could only ever be reached by pointer.
  const toggle = screen.getByRole("button", {
    name: /show the attempt trail/i,
  });
  // The chevron is turned by this attribute (aicalls.css), so what the reader
  // sees and what a screen reader hears are one fact rather than two that can
  // disagree. It is also why this stays a button and not a `Disclosure`: what
  // opens is the NEXT table row, which no element can contain from inside a
  // cell of the row above it.
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  await userEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  expect(await screen.findByText(/retry_on_5xx/)).toBeTruthy();
  expect(screen.getByText("Request payload")).toBeTruthy();
  expect(screen.getByText("Export as cert scenario")).toBeTruthy();
});

// The filter is a settings row now: the row draws the label and the Select is
// named BY it, so the combobox still says what it narrows while carrying no
// second name of its own — one visible label naming one control is the whole
// reason `control` is a function here.
it("names the task filter from its row, and stacks the trace under its own label", async () => {
  mount();
  const filter = await screen.findByRole("combobox", { name: "Task" });
  expect(filter.getAttribute("aria-label")).toBeNull();
  expect(filter.getAttribute("aria-labelledby")).toBeTruthy();
  // The trace is the subject of its row, not an answer beside it, so it stacks
  // under a label of its own rather than sharing the filter's.
  expect(screen.getByText("Recent calls")).toBeTruthy();
});

it("distinguishes capture disabled from a call without payload", async () => {
  mount(false, false);
  await userEvent.click(
    await screen.findByRole("button", { name: /show the attempt trail/i }),
  );
  expect(await screen.findByText(/Payload capture is off/)).toBeTruthy();
  cleanup();
  mount(true, false);
  await userEvent.click(
    await screen.findByRole("button", { name: /show the attempt trail/i }),
  );
  expect(
    await screen.findByText("No payload captured for this call."),
  ).toBeTruthy();
});

it("withholds the trace from a principal without the diagnostics read, and asks the server for nothing", async () => {
  // Withheld, not absent: an absent trace claims the installation made no model
  // calls. The card keeps its title and says whose record this is — and the list
  // read never fires, because the denial is already known.
  const { seen } = mount(true, true, { automation: ["read", "update"] });

  expect(
    await screen.findByText(/only an operator can read the per-call trace/i),
  ).toBeTruthy();
  expect(screen.getByText("AI call trace")).toBeTruthy();
  expect(seen.some((path) => path.includes("/ai/calls"))).toBe(false);
});
