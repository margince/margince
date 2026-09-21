/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProvidersStat, SpendStat } from "./ai-settings";

// Settings → AI, as one page: two readings above a strip that chooses between
// five bodies.
//
// The readings are what this file is mostly about. They follow DIFFERENT grants
// — spend on `ai_diagnostics:read`, the vendor keys on `ai_routing:read` — and
// each has three states a reader must be able to tell apart: answered, not
// theirs, and could not be read. The third is the one that used to say
// "Reading…" for ever.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const OPERATOR: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_diagnostics: ["read"],
  automation: ["read", "update"],
};
// Reaches the page on the automations read alone: no spend, no vendor keys.
// `automation:update` is deliberately held here and buys NEITHER reading — it
// is the grant the spend stat used to ask for, so this fixture fails the moment
// that gate comes back.
const NO_READINGS: GrantSpec = { automation: ["read", "update"] };
// The two grants the readings actually ride, and nothing else — the mirror of
// NO_READINGS, so the pair differs by exactly what is under test.
const BOTH_READINGS: GrantSpec = {
  ai_diagnostics: ["read"],
  ai_routing: ["read"],
};

const ROUTING = {
  profile: "eu_hosted",
  tiers: { premium: { provider: "anthropic", model: "claude-opus-4-8" } },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const USAGE = {
  days: [
    {
      date: "2026-09-01",
      tasks: [
        {
          task: "company.enrich",
          tier: "cheap_cloud",
          calls: 12,
          tokens_in: 1000,
          tokens_out: 200,
          cost_est_minor: 412,
        },
      ],
    },
  ],
  budget: {
    monthly_tokens: 1_000_000,
    spent_tokens: 214_000,
    band: "normal",
    currency: "USD",
  },
};

const KEYS = {
  providers: [
    { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY" },
    // Bound by the premium lane above and holding nothing — the join the
    // second reading reports.
    { provider: "anthropic", configured: false, env_var: "ANTHROPIC_API_KEY" },
  ],
};

// A month the runtime metered but could not price: the same shape, with no
// `cost_est_minor` on any task. Nothing in it is zero — the price is ABSENT,
// which is the difference the spend card's second line exists to say.
const UNPRICED_USAGE = {
  ...USAGE,
  days: [
    {
      date: "2026-09-01",
      tasks: [
        {
          task: "company.enrich",
          tier: "cheap_cloud",
          calls: 12,
          tokens_in: 1000,
          tokens_out: 200,
        },
      ],
    },
  ],
};

/** A backend answering every read this page makes, with per-route overrides. */
function backendFor(
  allow: GrantSpec,
  fail: { usage?: boolean; keys?: boolean } = {},
  routing: unknown = ROUTING,
  reads: { usage?: unknown; calls?: unknown[]; callsFail?: boolean } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (req.url.includes("/ai/usage")) {
      return fail.usage
        ? jsonResponse({ title: "upstream" }, 500)
        : jsonResponse(reads.usage ?? USAGE);
    }
    if (req.url.includes("/ai/provider-keys")) {
      return fail.keys
        ? jsonResponse({ title: "upstream" }, 500)
        : jsonResponse(KEYS);
    }
    if (req.url.includes("/ai/routing")) {
      return jsonResponse(routing);
    }
    if (req.url.includes("/ai/health")) {
      // Its own shape: the card reads `rungs`, and a catch-all that answered
      // the generic list envelope crashed it mid-render.
      return jsonResponse({ rungs: [] });
    }
    if (req.url.includes("/ai/calls")) {
      if (reads.callsFail) {
        return jsonResponse({ title: "upstream" }, 500);
      }
      return jsonResponse({
        data: reads.calls ?? [],
        page: { next_cursor: null, has_more: false },
        tasks: [],
        payload_capture_enabled: false,
      });
    }
    // The model sheet and the per-vendor lists the routing tab reaches for.
    return jsonResponse({ data: [] });
  });
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The two readings an operator opens AI settings for. They used to sit in a
// header above a five-tab strip; each tab is its own page now, so they ride the
// two pages they belong to — spend on usage, providers on models.
//
// The three cases below survived the strip's deletion because none of them was
// about the strip: what a WITHHELD reading says, what a FAILED one says, and
// that both answer at all. The three that went with it drove tab switching and
// the confirm dialog that guarded a routing draft across a shared address —
// behaviour that no longer exists, because leaving the routing page is an
// address change the app's own unsaved guard sees.
const BothStats = () => (
  <>
    <SpendStat />
    <ProvidersStat />
  </>
);

describe("the AI readings", () => {
  // A 200 that is not the routing document is an ABSENT read, not a crash.
  //
  // `tiers` and `embeddings` are both required of `AiRouting`, so the generated
  // type says they are there — but nothing validates a response, and the type is
  // a promise only the server keeps. Reading `Object.values(undefined)` threw
  // inside render, and a throw here does not cost the reading: the error
  // boundary sits above the shell, so the WHOLE settings page became "this view
  // no longer works". A server too old, a projection that lost a field and a
  // proxy answering something else all produce this body.
  //
  // The keyed count still answers, because it comes from a different read that
  // was fine — the page degrades to what it actually knows.
  it("says nothing about vendors when the routing read answers off-contract", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, {}, { profile: "eu_hosted" }));
    render(<BothStats />);

    expect(await screen.findByText("1 of 2")).toBeTruthy();
    expect(screen.queryByText(/bound, no key/)).toBeNull();
  });

  it("answers both readings", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<BothStats />);

    // Tokens are the budget the runtime actually enforces, so they ARE the
    // figure and the unit is in the label; the money is the estimate priced on
    // read and rides the line under it.
    expect(screen.getByText(en["aiSettings.spend.label"])).toBeTruthy();
    expect(await screen.findByText("214k of 1m")).toBeTruthy();
    expect(screen.getByText(/US\$4\.12 spent/)).toBeTruthy();
    // One vendor keyed OUT OF the vendors this installation knows about, and
    // the one the routing binds without a key named as the thing to act on.
    expect(await screen.findByText("1 of 2")).toBeTruthy();
    expect(await screen.findByText(/1 bound, no key/)).toBeTruthy();
  });

  // Withheld, not absent. An absent spend reading would claim this installation
  // had spent nothing, which is a statement about the DATA where the truth is
  // only about who may read it.
  //
  // The withheld text is ALSO what both stats say while /me is still in flight
  // — every capability predicate reads false until the snapshot lands — so
  // finding it proves nothing on its own. The grant has to be observed being
  // read, which is what the paired ANSWERED case below is for: the same two
  // stats, the same wiring, one grant apart. Assert both from one fixture pair
  // or the refusal is vacuous.
  it("says a reading is withheld rather than dropping it", async () => {
    vi.stubGlobal("fetch", backendFor(NO_READINGS));
    const { unmount } = render(<BothStats />);

    expect(await screen.findAllByText("Restricted")).toHaveLength(2);
    unmount();
    cleanup();

    // The positive control, and the whole proof: swap ONLY the grants and the
    // same two stats answer. A gate asking for the wrong object leaves this
    // half showing "Restricted" and fails here.
    vi.stubGlobal("fetch", backendFor(BOTH_READINGS));
    render(<BothStats />);

    expect(await screen.findByText("214k of 1m")).toBeTruthy();
    expect(await screen.findByText("1 of 2")).toBeTruthy();
    expect(screen.queryByText("Restricted")).toBeNull();
  });

  // A read that FAILED and a read that has not arrived are different facts, and
  // only one of them resolves by waiting: "Reading…" over a failed request is a
  // page that says it is still working for ever.
  it("says a reading could not be read rather than reading for ever", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, { usage: true, keys: true }));
    render(<BothStats />);

    await waitFor(() =>
      expect(screen.getAllByText("Unavailable")).toHaveLength(2),
    );
    expect(screen.queryByText("Loading")).toBeNull();
  });

  // A month nothing priced is not a month that cost nothing. The estimate line
  // used to be absent there, which reads as free — the one claim this product
  // must never make by accident.
  it("says a month is not priced rather than leaving the estimate out", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, {}, ROUTING, { usage: UNPRICED_USAGE }),
    );
    render(<SpendStat />);

    expect(await screen.findByText("214k of 1m")).toBeTruthy();
    expect(screen.getByText(en["aiSettings.spend.notPriced"])).toBeTruthy();
    expect(screen.queryByText(/spent/)).toBeNull();
  });

  // A vendor bound and never reached is its own fact, and it used to fall
  // through to a detail line with nothing in it at all.
  it("says a runtime has never been called rather than drawing an empty line", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<ProvidersStat />);

    expect(
      await screen.findByText(en["aiSettings.providers.neverCalled"]),
    ).toBeTruthy();
  });

  // The two halves qualify the SAME reading, so they share one line — and the
  // second one's case is the first one's to decide.
  it("puts what is broken and when a vendor was last reached on one line", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, {}, ROUTING, {
        calls: [{ id: "call-1", occurred_at: new Date().toISOString() }],
      }),
    );
    render(<ProvidersStat />);

    const missing = await screen.findByText("1 bound, no key");
    const line = missing.parentElement;
    expect(line?.textContent).toMatch(/1 bound, no key · last call/);
    expect(
      screen.queryByText(en["aiSettings.providers.neverCalled"]),
    ).toBeNull();
  });

  // "Never called" is a claim about the INSTALLATION, and a reader who may not
  // read diagnostics has given this card no evidence for it. The keys are a
  // different grant, so the count still answers — the trace line is what goes
  // quiet.
  it("says nothing about the last call when the trace is not this reader's", async () => {
    vi.stubGlobal("fetch", backendFor({ ai_routing: ["read"] }));
    render(<ProvidersStat />);

    expect(await screen.findByText("1 of 2")).toBeTruthy();
    expect(
      screen.queryByText(en["aiSettings.providers.neverCalled"]),
    ).toBeNull();
    expect(screen.queryByText(/last call/i)).toBeNull();
  });

  // A BROKEN trace read is not a quiet one. Waiting will not fix it, so the
  // line says so rather than leaving a reader to read silence as "nothing to
  // report" — the same distinction the two readings above it already make.
  it("says the call trace could not be read rather than going quiet", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, {}, ROUTING, { callsFail: true }),
    );
    render(<ProvidersStat />);

    expect(
      await screen.findByText(en["aiSettings.providers.traceFailed"]),
    ).toBeTruthy();
    expect(
      screen.queryByText(en["aiSettings.providers.neverCalled"]),
    ).toBeNull();
  });
});
