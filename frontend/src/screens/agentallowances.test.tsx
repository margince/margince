/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { AgentAllowancesCard } from "./agentallowances";
import { defaultAgentAllowances } from "./agentallowances.fixtures";

// Settings → AI usage: what one passport may do per window. An admin changes
// each value, and a value past its bounds is refused in the box. A saved
// window length is reported as waiting for its reset.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const EDITOR: GrantSpec = { installation_settings: ["read", "update"] };
const READER: GrantSpec = { installation_settings: ["read"] };

function backendFor(
  allow: GrantSpec,
  allowances: object = defaultAgentAllowances,
) {
  let agent_allowances = { ...allowances };
  const patches: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url, "http://localhost");
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (url.pathname.endsWith("/installation/settings")) {
        if (req.method === "PATCH") {
          const patch = await req.json();
          patches.push(patch);
          agent_allowances = { ...agent_allowances, ...(patch as object) };
        }
        return jsonResponse({ agent_allowances });
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  return { fetchMock, patches };
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

describe("AgentAllowancesCard", () => {
  it.each([
    ["agent_reads_per_window", "5000", "99"],
    ["agent_writes_per_window", "10", "5001"],
    ["agent_egress_per_window", "5", "0"],
    ["agent_calls_per_window", "50", "20001"],
    ["agent_window_hours", "12", "169"],
  ] as const)(
    "saves %s and refuses past its bounds",
    async (property, ok, bad) => {
      const backend = backendFor(EDITOR);
      vi.stubGlobal("fetch", backend.fetchMock);
      const user = userEvent.setup();
      render(<AgentAllowancesCard />);

      const box = await screen.findByTestId<HTMLInputElement>(
        `allowance-${property}`,
      );
      expect(box.value).toBe(String(defaultAgentAllowances[property]));
      await user.clear(box);
      await user.type(box, `${bad}{Enter}`);
      expect(
        await screen.findByText(translate("en", "operations.refusal")),
      ).toBeTruthy();
      expect(backend.patches).toEqual([]);

      await user.clear(box);
      await user.type(box, `${ok}{Enter}`);
      await waitFor(() =>
        expect(backend.patches).toEqual([{ [property]: Number(ok) }]),
      );
    },
  );

  it("reads the window length in words", async () => {
    vi.stubGlobal("fetch", backendFor(EDITOR).fetchMock);
    render(<AgentAllowancesCard />);

    const window = await screen.findByTestId("allowance-agent_window_hours");
    expect(window).toHaveAccessibleDescription(
      expect.stringContaining("= 1 day"),
    );
  });

  it("says when a saved window length takes over, and cancels it on the length in force", async () => {
    const backend = backendFor(EDITOR, {
      ...defaultAgentAllowances,
      agent_window_next_hours: 12,
      agent_window_next_starts_at: "2026-10-15T00:00:00Z",
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentAllowancesCard />);

    const window = await screen.findByTestId<HTMLInputElement>(
      "allowance-agent_window_hours",
    );
    expect(window.value).toBe("12");

    expect(
      await screen.findByText(translate("en", "agentAllowances.pending.title")),
    ).toBeTruthy();
    expect(screen.getByText(/Each window lasts 12 hours/)).toBeTruthy();
    await user.clear(window);
    await user.type(window, "24{Enter}");
    await waitFor(() =>
      expect(backend.patches).toEqual([{ agent_window_hours: 24 }]),
    );
  });

  it("shows the fixed MCP limits and when the window resets", async () => {
    vi.stubGlobal("fetch", backendFor(EDITOR).fetchMock);
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByText(/240 calls and 30 stream opens/),
    ).toBeTruthy();
    expect(
      screen.getByText(/Every allowance resets .*2026.* UTC/),
    ).toBeTruthy();
  });

  it("is read-only to a seat without the update grant", async () => {
    vi.stubGlobal("fetch", backendFor(READER).fetchMock);
    render(<AgentAllowancesCard />);

    expect(
      await screen.findByTestId("allowance-agent_writes_per_window"),
    ).toBeDisabled();
    expect(
      screen.getByText(translate("en", "operations.adminOnly")),
    ).toBeTruthy();
  });

  it("draws nothing for a seat that cannot read the settings", async () => {
    vi.stubGlobal("fetch", backendFor({}).fetchMock);
    const { container } = render(<AgentAllowancesCard />);

    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});

it("redraws the usage bars against a saved allowance", async () => {
  const backend = backendFor(EDITOR);
  vi.stubGlobal("fetch", backend.fetchMock);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const user = userEvent.setup();
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <AgentAllowancesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );

  const box = await screen.findByTestId<HTMLInputElement>(
    "allowance-agent_writes_per_window",
  );
  await user.clear(box);
  await user.type(box, "50{Enter}");

  await waitFor(() =>
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["agent-usage"] }),
  );
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["passports"] });
});
