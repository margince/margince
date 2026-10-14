/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { AiProviderKeysCard } from "./ai-provider-keys";

// The providers status line at the top of the Providers panel, rendered through
// the panel that owns it, on its real reads.

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
const ROUTING = {
  profile: "eu_hosted",
  tiers: { premium: { provider: "anthropic", model: "claude-opus-4-8" } },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const KEYS = {
  providers: [
    {
      provider: "gemini",
      configured: true,
      env_var: "GEMINI_API_KEY",
      usable: true,
    },
    // Bound by the premium lane above and holding nothing — the join the
    // second reading reports.
    {
      provider: "anthropic",
      configured: false,
      env_var: "ANTHROPIC_API_KEY",
      usable: false,
    },
  ],
};

/** A backend answering every read this page makes, with per-route overrides. */
function backendFor(
  allow: GrantSpec,
  routing: unknown = ROUTING,
  reads: {
    calls?: unknown[];
    callsFail?: boolean;
    keys?: unknown;
    served?: string[];
  } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (req.url.includes("/ai/provider-keys")) {
      return jsonResponse(reads.keys ?? KEYS);
    }
    if (req.url.includes("/ai/provider-health")) {
      return jsonResponse({
        providers: [],
        configured: reads.served ?? ["anthropic", "gemini"],
      });
    }
    if (req.url.includes("/ai/routing")) {
      return jsonResponse(routing);
    }
    if (req.url.includes("/ai/health")) {
      // Its own shape: the card reads `rungs`, and a catch-all that answered
      // the generic list envelope crashed it mid-render.
      return jsonResponse({ rungs: [] });
    }
    if (req.url.includes("/ai/call-stats")) {
      return jsonResponse({ rows: [] });
    }
    if (req.url.includes("/ai/calls")) {
      if (reads.callsFail) {
        return jsonResponse({ title: "upstream" }, 500);
      }
      return jsonResponse({
        data: reads.calls ?? [],
        page: { next_cursor: null, has_more: false },
        tasks: [],
        task_options: [],
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

// The line, once the panel has read the keys.
async function statusLine(): Promise<HTMLElement> {
  return screen.findByTestId("ai-providers-status");
}

describe("the providers status line", () => {
  // A 200 that is not the routing document is an ABSENT read, not a crash:
  // nothing validates a response, and `Object.values(undefined)` threw inside
  // render. The keyed count comes from a different read and still answers.
  it("says nothing about vendors when the routing read answers off-contract", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, { profile: "eu_hosted" }));
    render(<AiProviderKeysCard />);

    const line = await statusLine();
    expect(within(line).getByText("Keys held: 1 of 2")).toBeTruthy();
    expect(line.textContent).not.toContain("bound, no key");
  });

  it("counts the keyed vendors and names the bound one without a key", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<AiProviderKeysCard />);

    // Out of the vendors this installation knows about, and the one the routing
    // binds without a key named as the thing to act on.
    const line = await statusLine();
    expect(within(line).getByText("Keys held: 1 of 2")).toBeTruthy();
    expect(await within(line).findByText("1 bound, no key")).toBeTruthy();
  });

  // The decision model is bound apart from the tiers, and TypeSafe's own API
  // demands its key like any vendor; a jev_compatible key is optional, so a
  // lane on it counts toward nothing missing.
  it("counts the decision model's key as bound, unless that key is optional", async () => {
    const keys = (optional: boolean, provider: string) => ({
      providers: [
        ...KEYS.providers,
        {
          provider,
          configured: false,
          env_var: "K",
          usable: optional,
          optional,
        },
      ],
    });
    const routing = (provider: string) => ({
      ...ROUTING,
      decisions: { provider, model: "m" },
    });
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, routing("jev"), { keys: keys(false, "jev") }),
    );
    const official = render(<AiProviderKeysCard />);
    expect(
      await within(await statusLine()).findByText("2 bound, no key"),
    ).toBeTruthy();
    official.unmount();

    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, routing("jev_compatible"), {
        keys: keys(true, "jev_compatible"),
      }),
    );
    render(<AiProviderKeysCard />);
    expect(
      await within(await statusLine()).findByText("1 bound, no key"),
    ).toBeTruthy();
  });

  // A vendor bound and never reached is its own fact, not an empty line.
  it("says a runtime has never been called rather than drawing an empty line", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR));
    render(<AiProviderKeysCard />);

    expect(
      await within(await statusLine()).findByText(
        en["aiSettings.providers.neverCalled"],
      ),
    ).toBeTruthy();
  });

  it("puts what is broken and when a vendor was last reached on one line", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, ROUTING, {
        calls: [{ id: "call-1", occurred_at: new Date().toISOString() }],
      }),
    );
    render(<AiProviderKeysCard />);

    const line = await statusLine();
    expect(await within(line).findByText(/^last call/)).toBeTruthy();
    expect(line.textContent).toMatch(
      /Keys held: 1 of 2.*1 bound, no key.*last call/,
    );
    expect(line.textContent).not.toContain(
      en["aiSettings.providers.neverCalled"],
    );
  });

  // "Never called" is a claim about the INSTALLATION, and a reader who may not
  // read diagnostics has given this line no evidence for it.
  it("says nothing about the last call when the trace is not this reader's", async () => {
    vi.stubGlobal("fetch", backendFor({ ai_routing: ["read"] }));
    render(<AiProviderKeysCard />);

    const line = await statusLine();
    expect(within(line).getByText("Keys held: 1 of 2")).toBeTruthy();
    expect(line.textContent).not.toContain(
      en["aiSettings.providers.neverCalled"],
    );
    expect(line.textContent).not.toMatch(/last call/i);
  });

  // A BROKEN trace read is not a quiet one: waiting will not fix it.
  it("says the call trace could not be read rather than going quiet", async () => {
    vi.stubGlobal("fetch", backendFor(OPERATOR, ROUTING, { callsFail: true }));
    render(<AiProviderKeysCard />);

    expect(
      await within(await statusLine()).findByText(
        en["aiSettings.providers.traceFailed"],
      ),
    ).toBeTruthy();
  });

  // A boot that cannot serve its binding may run on the offline fake. The line
  // then says where answers come from, not when a vendor was reached.
  it("says the answers come from the built-in test provider when the process serves the fake", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(OPERATOR, ROUTING, {
        calls: [{ id: "call-1", occurred_at: new Date().toISOString() }],
        served: ["fake"],
      }),
    );
    render(<AiProviderKeysCard />);

    const line = await statusLine();
    expect(
      await within(line).findByText(en["aiSettings.providers.standIn"]),
    ).toBeTruthy();
    expect(line.textContent).not.toMatch(/last call/);
  });
});
