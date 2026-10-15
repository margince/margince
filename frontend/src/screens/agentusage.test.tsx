/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { AgentUsageByUserCard } from "./agentusage";

// Settings → AI usage: how much each user's agents have used this window. The
// card answers how much and whose, and is withheld from a seat that may not
// read AI usage.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const counter = (used: number, limit: number) => ({ used, limit, released: 0 });

const PAGE = {
  resets_at: "2026-10-15T00:00:00Z",
  data: [
    {
      user_id: "u-minh",
      display_name: "Minh Tran",
      passport_count: 2,
      writes: counter(400, 400),
      egress: counter(20, 40),
      calls: counter(980, 2_000),
      state: "stopped",
    },
  ],
};

function backendFor(allow: GrantSpec, usage: () => Response) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url, "http://localhost");
    if (url.pathname.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (url.pathname.endsWith("/ai/agent-usage")) {
      return usage();
    }
    throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
  });
}

function render(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AgentUsageByUserCard", () => {
  it("shows each user's usage of every allowance and their state", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ ai_diagnostics: ["read"] }, () => jsonResponse(PAGE)),
    );
    const user = userEvent.setup();
    render(<AgentUsageByUserCard />);

    const row = (await screen.findByText("Minh Tran")).closest("tr");
    if (!row) {
      throw new Error("the user row is not rendered");
    }
    // The allowance closest to its limit, and the state, fit the row.
    expect(within(row).getByText("400 of 400")).toBeTruthy();
    expect(within(row).getByText("Stopped")).toBeTruthy();
    expect(within(row).getAllByRole("meter")).toHaveLength(1);

    await user.click(
      screen.getByRole("button", {
        name: "Show this window’s usage for Minh Tran",
      }),
    );
    expect(screen.getByText("980 of 2,000")).toBeTruthy();
    expect(screen.getAllByRole("meter")).toHaveLength(4);
    expect(
      screen.getByText(/Every allowance resets .*2026.* UTC/),
    ).toBeTruthy();
  });

  it("asks for nothing and says why when the seat may not read AI usage", async () => {
    const fetchMock = backendFor({}, () => jsonResponse(PAGE));
    vi.stubGlobal("fetch", fetchMock);
    render(<AgentUsageByUserCard />);

    expect(
      await screen.findByText(translate("en", "agentUsage.withheld")),
    ).toBeTruthy();
    const asked = fetchMock.mock.calls.map(([input]) =>
      String(input instanceof Request ? input.url : input),
    );
    expect(asked.some((url) => url.includes("/ai/agent-usage"))).toBe(false);
  });

  it("says so when no user has a live passport", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ ai_diagnostics: ["read"] }, () =>
        jsonResponse({ ...PAGE, data: [] }),
      ),
    );
    render(<AgentUsageByUserCard />);

    expect(
      await screen.findByText(translate("en", "agentUsage.empty")),
    ).toBeTruthy();
  });

  it("reports counters that cannot be read", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ ai_diagnostics: ["read"] }, () =>
        jsonResponse({ title: "Service unavailable", status: 503 }, 503),
      ),
    );
    render(<AgentUsageByUserCard />);

    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});
