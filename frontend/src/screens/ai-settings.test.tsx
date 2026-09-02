/** @vitest-environment jsdom */
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
import { type GrantSpec, meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { AiSettingsTab } from "./ai-settings";

// Settings → AI, as one page: two readings above a strip that chooses between
// five bodies.
//
// The readings are what this file is mostly about. They follow DIFFERENT grants
// — spend on `automation:update`, the vendor keys on `ai_routing:read` — and
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
  automation: ["read", "update"],
};
// Reaches the page on the automations read alone: no spend, no vendor keys.
const NO_READINGS: GrantSpec = { automation: ["read"] };

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
      return jsonResponse(ROUTING);
    }
    if (req.url.includes("/ai/certification")) {
      // Its own shape too, for the same reason /ai/health has one: the card
      // reads `jobs`, and the generic list envelope below would crash it.
      return jsonResponse({
        binding_state: "bound",
        runs_per_example: 3,
        jobs: [],
      });
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

describe("AiSettingsTab", () => {
  it("answers both readings before a tab is chosen", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<AiSettingsTab />);

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
  it("says a reading is withheld rather than dropping it", async () => {
    vi.stubGlobal("fetch", backendFor(NO_READINGS));
    render(<AiSettingsTab />);

    expect(await screen.findAllByText("Not yours to see")).toHaveLength(2);
  });

  // A read that FAILED and a read that has not arrived are different facts, and
  // only one of them resolves by waiting: "Reading…" over a failed request is a
  // page that says it is still working for ever.
  it("says a reading could not be read rather than reading for ever", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, { usage: true, keys: true }));
    render(<AiSettingsTab />);

    await waitFor(() =>
      expect(screen.getAllByText("Could not be read")).toHaveLength(2),
    );
    expect(screen.queryByText("Reading…")).toBeNull();
  });

  it("opens on routing and swaps one body at a time", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<AiSettingsTab />);

    // The lane the routing binds, which is the Routing body.
    expect(await screen.findByText("premium")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Providers" }));
    expect(await screen.findByText("Model provider keys")).toBeTruthy();
    // And the body it replaced is GONE rather than merely scrolled past —
    // each is its own read, and keeping four warm behind a strip nobody is
    // looking at spends the installation's read budget on absent screens.
    expect(screen.queryByText("Routing lanes")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Logs" }));
    expect(await screen.findByText("AI call trace")).toBeTruthy();
  });

  // The routing draft is a document held in the card, and the strip above it is
  // a place a reader MOVES — which the app's own unsaved guard cannot see,
  // because it watches addresses and every tab here shares one.
  it("asks before a tab change would discard routing edits", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<AiSettingsTab />);

    const lane = await screen.findByTestId("ai-routing-tier-premium");
    await user.click(within(lane).getByRole("button", { name: /change/i }));
    const model = within(lane).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "claude-opus-5");

    await user.click(screen.getByRole("button", { name: "Usage" }));
    // Still on Routing, with the question in front of the reader.
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/have not been saved/i)).toBeTruthy();
    expect(screen.getByText("Routing lanes")).toBeTruthy();

    await user.click(within(dialog).getByRole("button", { name: /discard/i }));
    expect(await screen.findByText("AI usage & budget")).toBeTruthy();
  });

  // And a form that has been SAVED is no longer unsaved.
  //
  // The re-seed guard refuses to touch a dirty form, and a successful write
  // leaves the form dirty by that measure until the draft is re-seeded from what
  // the server returned. Without that, the page went on offering to discard
  // edits that had already landed — the worst shape this question can take,
  // because a reader who says yes loses nothing and learns to distrust it.
  it("refreshes how-well-it-performs when the binding is saved", async () => {
    // Both cards sit in this tab, and the lower one reports on whichever models
    // are bound. Without invalidation it keeps answering for the binding that
    // was just replaced — a reliability figure for a model the reader has
    // stopped using, on the same screen where they stopped using it.
    const user = userEvent.setup({ delay: null });
    let certReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req = new Request(input, init);
        if (req.url.includes("/ai/certification")) {
          certReads += 1;
          return jsonResponse({
            binding_state: "bound",
            jobs: [],
          });
        }
        if (req.url.includes("/ai/routing")) {
          return jsonResponse(ROUTING);
        }
        return backendFor(OPERATOR)(input, init);
      }),
    );
    render(<AiSettingsTab />);

    const lane = await screen.findByTestId("ai-routing-tier-premium");
    await waitFor(() => expect(certReads).toBeGreaterThan(0));
    const before = certReads;

    await user.click(within(lane).getByRole("button", { name: /change/i }));
    await user.click(screen.getByRole("button", { name: /save routing/i }));

    await waitFor(() => expect(certReads).toBeGreaterThan(before));
  });

  it("stops asking once the edits have been saved", async () => {
    const user = userEvent.setup();
    const saved = {
      ...ROUTING,
      tiers: { premium: { provider: "anthropic", model: "claude-opus-5" } },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/ai/routing")) {
          // The PUT answers with the stored document, which is what the form
          // re-seeds from.
          return jsonResponse(req.method === "PUT" ? saved : ROUTING);
        }
        return backendFor(OPERATOR)(input, init);
      }),
    );
    render(<AiSettingsTab />);

    const lane = await screen.findByTestId("ai-routing-tier-premium");
    await user.click(within(lane).getByRole("button", { name: /change/i }));
    const model = within(lane).getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "claude-opus-5");
    await user.click(screen.getByRole("button", { name: /save routing/i }));
    await screen.findByText(/Routing saved/i);

    // Leaving now is an ordinary move, not a question.
    await user.click(screen.getByRole("button", { name: "Usage" }));
    expect(await screen.findByText("AI usage & budget")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
