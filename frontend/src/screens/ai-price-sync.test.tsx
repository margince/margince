/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { ModelPricesCard } from "./ai-price-sync";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const NOW = new Date("2026-10-02T12:00:00Z");
const ADMIN: GrantSpec = { ai_model_rate: ["read", "create", "update"] };
const READER: GrantSpec = { ai_model_rate: ["read"] };

const line = (
  provider: string,
  outcome: string,
  counts: Record<string, number> = {},
) => ({
  provider,
  outcome,
  updated: 0,
  unchanged: 0,
  added: 0,
  kept: 0,
  models: [],
  unlisted: [],
  ...counts,
});

const SYNCED = {
  auto_sync: true,
  last_run: {
    ran_at: "2026-10-02T10:00:00Z",
    trigger: "scheduled",
    report: {
      providers: [
        line("gemini", "updated", { updated: 3, added: 2 }),
        line("anthropic", "unchanged", { kept: 1 }),
        line("ollama", "not_available"),
      ],
    },
  },
};

function backend(allow: GrantSpec, state: unknown) {
  const puts: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) return jsonResponse(meFixture({ allow }));
      if (req.url.endsWith("/ai/price-sync")) {
        if (req.method === "PUT") {
          const body = await req.json();
          puts.push(body);
          return jsonResponse({ ...(state as object), ...body });
        }
        return jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return { puts };
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ModelPricesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("ModelPricesCard", () => {
  it("says it has not synced before the first run", async () => {
    backend(ADMIN, { auto_sync: true });
    mount();
    expect(await screen.findByText("Not synced yet")).toBeInTheDocument();
    expect(
      screen.getByText("Sources: models.dev · OpenRouter"),
    ).toBeInTheDocument();
  });

  it("shows when it last synced and what it did per vendor", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
    backend(ADMIN, SYNCED);
    mount();
    expect(await screen.findByText("Last synced 2 h ago")).toBeInTheDocument();
    expect(screen.getByText("2 models added")).toBeInTheDocument();
    expect(screen.getByText("1 hand-set price kept")).toBeInTheDocument();
    expect(screen.queryByText(/Ollama/)).not.toBeInTheDocument();
  });

  it("writes the switch when an admin turns auto-sync off", async () => {
    const { puts } = backend(ADMIN, { auto_sync: true });
    const user = userEvent.setup();
    mount();
    await user.click(
      await screen.findByRole("switch", { name: "Auto-sync daily" }),
    );
    await waitFor(() => expect(puts).toEqual([{ auto_sync: false }]));
  });

  it("refuses the switch and hides Refresh now from a reader", async () => {
    backend(READER, { auto_sync: true });
    mount();
    const toggle = await screen.findByRole("switch", {
      name: "Auto-sync daily",
    });
    expect(toggle).toBeDisabled();
    expect(
      screen.getByText("Your role cannot change this."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Refresh model prices" }),
    ).not.toBeInTheDocument();
  });

  // A run that adds a model creates its row, so a seat that may only update
  // would press the button into a refusal.
  it("offers Refresh now only to a seat that may both add and update prices", async () => {
    backend({ ai_model_rate: ["read", "update"] }, { auto_sync: true });
    mount();
    await screen.findByRole("switch", { name: "Auto-sync daily" });
    expect(
      screen.queryByRole("button", { name: "Refresh model prices" }),
    ).not.toBeInTheDocument();
  });

  it("withholds the card from a seat without the price sheet", async () => {
    backend({}, { auto_sync: true });
    mount();
    expect(
      await screen.findByText("Model prices are not yours to see."),
    ).toBeInTheDocument();
  });
});
