// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The routing card's fixtures, shared by its suites: the grant, the stubbed
// server and the reader's moves. One copy, so no suite is tested against a
// routing document the others never saw.

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { expect, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";

// Settings → AI → Model routing: which vendor this installation's text is sent
// to. The server is the RBAC authority; this screen mirrors it by disabling
// (never hiding) the save for a reader who may not change it, so an operator
// can still SEE the binding they are asking somebody else to change.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ETag: '"routing-v1"' },
  });
}

// Naming the grant rather than a role keeps the fixture honest about what the
// screen actually asks for.
export const ROUTING_EDITOR: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_budget: ["read"],
};
export const ROUTING_READER: GrantSpec = { ai_routing: ["read"] };

/** What PUT /ai/routing carries, as these tests read it back. */
export type CapturedRouting = {
  profile: string;
  tiers: Record<
    string,
    { provider: string; model: string; base_url?: string; routing?: unknown }
  >;
  embeddings: {
    provider: string;
    model: string;
    base_url?: string;
    dimensions?: number;
  };
  decisions?: { provider: string; model: string; base_url?: string };
};

// The price sheet, which is also the catalogue this card offers from: a model
// outside it serves calls and reports UNPRICED. The full wire row, prices
// included — the picker renders them, so a fixture without them is not the
// shape this screen receives.
export function sheetRow(
  provider: string,
  model_id: string,
  lane: "chat" | "embeddings" | "decisions",
  input_per_mtok: string,
  output_per_mtok: string,
) {
  return {
    provider,
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}

export const SHEET = [
  sheetRow("gemini", "gemini-3.5-flash", "chat", "1.50", "9.00"),
  sheetRow("gemini", "gemini-3.1-flash-lite", "chat", "0.25", "1.50"),
  sheetRow("gemini", "gemini-3.1-pro-preview", "chat", "2.00", "12.00"),
  sheetRow("gemini", "gemini-embedding-001", "embeddings", "0.15", "0"),
  sheetRow("anthropic", "claude-opus-4-8", "chat", "5.00", "25.00"),
  // A decision model bills input alone, so the sheet leaves output blank.
  sheetRow("openrouter_decision", "jev-classify", "decisions", "0.40", ""),
];

// Which vendors hold a credential. `anthropic` is bound by nothing in BOUND,
// so its absence never lights a row: the pill follows the ROUTING, and a vendor
// nobody points at is not this installation's problem.
export const PROVIDER_KEYS = [
  { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY" },
  { provider: "anthropic", configured: false, env_var: "ANTHROPIC_API_KEY" },
];

export const BOUND = {
  profile: "eu_hosted",
  tiers: {
    premium: { provider: "gemini", model: "gemini-3.5-flash" },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

// What an installation that has bound nothing answers: an empty tier map, not
// null — the contract is explicit that it says so with `{}`.
export const UNBOUND = {
  profile: "",
  tiers: {},
  embeddings: { provider: "", model: "" },
};

export const VENDOR_MODELS: Record<string, unknown> = {
  gemini: {
    provider: "gemini",
    models: [
      // Newer than anything on the sheet: the model a reader came looking for.
      {
        id: "gemini-4.0-flash",
        display_name: "Gemini 4.0 Flash",
        lane: "chat",
      },
      {
        id: "gemini-3.5-flash",
        display_name: "Gemini 3.5 Flash",
        lane: "chat",
      },
      { id: "gemini-embedding-001", lane: "embeddings" },
    ],
  },
  anthropic: { provider: "anthropic", models: [], unavailable: "no_key" },
};

export function backendFor(
  allow: GrantSpec,
  routing: unknown = BOUND,
  {
    sheetStatus = 200,
    providerKeys = PROVIDER_KEYS,
  }: {
    sheetStatus?: number;
    providerKeys?: readonly {
      provider: string;
      configured: boolean;
      env_var: string;
    }[];
  } = {},
) {
  let stored = routing;
  let revision = "routing-v1";
  // Typed as the document this endpoint takes, so an assertion can read a field
  // off it without an unchecked cast at every call site. The stub still stores
  // whatever arrives — the type is a claim about the ENDPOINT, not a check on
  // the body, and a test asserting the wrong shape fails on the assertion.
  let capturedPut: CapturedRouting | null = null;
  const fetchMock = vi.fn(
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: one stubbed server answering every endpoint the card calls, route by route; it moved here unchanged from the suite, where test files carry no complexity cap
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/ai/available-models/")) {
        // The lane rides along as a query parameter, so the provider is the
        // path segment before it — the vendor is what this stub answers for.
        const provider = req.url
          .split("/ai/available-models/")[1]
          .split("?")[0];
        return jsonResponse(
          VENDOR_MODELS[provider] ?? { provider, models: [] },
        );
      }
      if (req.url.includes("/ai/provider-keys")) {
        return jsonResponse({ providers: providerKeys });
      }
      if (req.url.includes("/ai-model-rates")) {
        return sheetStatus === 200
          ? jsonResponse({ data: SHEET })
          : jsonResponse({ title: "forbidden" }, sheetStatus);
      }
      if (req.url.includes("/ai/routing/preview"))
        return jsonResponse({
          current_version: revision,
          features: [],
          unused_tiers: [],
        });
      if (req.url.includes("/ai/routing")) {
        if (req.method === "PUT") {
          capturedPut = (await req.json()) as CapturedRouting;
          stored = capturedPut;
        }
        const response = jsonResponse(stored);
        response.headers.set("ETag", `"${revision}"`);
        return response;
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return {
    fetchMock,
    externalChange: () => {
      stored = { ...BOUND, profile: "best_effort" };
      revision = "routing-v2";
    },
    getCapturedPut: (): CapturedRouting | null => capturedPut,
  };
}

/** Opens one lane's fields, the way a reader does. */
export async function openLane(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
) {
  const lane = screen.getByTestId(testId);
  await user.click(within(lane).getByRole("button", { name: /change/i }));
  return lane;
}

export const render = (
  ui: ReactNode,
  locale: Locale = "en",
): RenderResult & { client: QueryClient } => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const result = rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...result, client };
};

/** Previews, then saves, and hands back what the PUT carried. */
export async function previewAndSave(
  user: ReturnType<typeof userEvent.setup>,
  backend: ReturnType<typeof backendFor>,
) {
  await user.click(screen.getByRole("button", { name: /preview effects/i }));
  const save = screen.getByRole("button", { name: /save routing/i });
  await waitFor(() => expect(save).not.toBeDisabled());
  await user.click(save);
  await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
  return backend.getCapturedPut();
}
