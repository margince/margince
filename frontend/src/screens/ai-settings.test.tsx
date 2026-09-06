/** @vitest-environment jsdom */
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

/** A backend answering every read this page makes, with per-route overrides. */
function backendFor(
  allow: GrantSpec,
  fail: { usage?: boolean; keys?: boolean } = {},
  routing: unknown = ROUTING,
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
        : jsonResponse(USAGE);
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
      return jsonResponse({
        data: [],
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

    expect(await screen.findByText("1 keyed")).toBeTruthy();
    expect(screen.queryByText(/bound with no key/)).toBeNull();
  });

  it("answers both readings", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<BothStats />);

    // Tokens are the budget the runtime actually enforces; the money is the
    // estimate priced on read, and it is a second line rather than the figure.
    expect(await screen.findByText(/214,000 of 1,000,000 tokens/)).toBeTruthy();
    expect(screen.getByText(/estimated/)).toBeTruthy();
    // One vendor keyed, and the one the routing binds without a key named as
    // the thing to act on.
    expect(await screen.findByText("1 keyed")).toBeTruthy();
    expect(await screen.findByText(/1 bound with no key/)).toBeTruthy();
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

    expect(await screen.findAllByText("Not yours to see")).toHaveLength(2);
    unmount();
    cleanup();

    // The positive control, and the whole proof: swap ONLY the grants and the
    // same two stats answer. A gate asking for the wrong object leaves this
    // half showing "Not yours to see" and fails here.
    vi.stubGlobal("fetch", backendFor(BOTH_READINGS));
    render(<BothStats />);

    expect(await screen.findByText(/214,000 of 1,000,000 tokens/)).toBeTruthy();
    expect(await screen.findByText("1 keyed")).toBeTruthy();
    expect(screen.queryByText("Not yours to see")).toBeNull();
  });

  // A read that FAILED and a read that has not arrived are different facts, and
  // only one of them resolves by waiting: "Reading…" over a failed request is a
  // page that says it is still working for ever.
  it("says a reading could not be read rather than reading for ever", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, { usage: true, keys: true }));
    render(<BothStats />);

    await waitFor(() =>
      expect(screen.getAllByText("Could not be read")).toHaveLength(2),
    );
    expect(screen.queryByText("Reading…")).toBeNull();
  });
});
