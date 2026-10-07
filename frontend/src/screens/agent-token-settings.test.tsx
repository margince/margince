/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider, translate } from "../i18n";
import { AgentConnectionsCard } from "./agent-token-settings";

// Settings → Sign-in and apps: how long a connected agent's passport lives.
// Every role reads it; installation_settings:update changes it, and a value
// past the passport's own ceiling is refused in the box before the request.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SETTINGS_EDITOR: GrantSpec = {
  installation_settings: ["read", "update"],
};
const SETTINGS_READER: GrantSpec = { installation_settings: ["read"] };

function backendFor(allow: GrantSpec) {
  let state = {
    oauth_access_token_ttl_minutes: 43_200,
  };
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
          state = { ...state, ...(patch as object) };
        }
        return jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
    },
  );
  return { fetchMock, patches };
}

function render(
  node: ReactNode,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
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

describe("AgentConnectionsCard", () => {
  it("saves a shorter passport lifetime on Enter", async () => {
    const backend = backendFor(SETTINGS_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentConnectionsCard />);

    const ttl = await screen.findByTestId<HTMLInputElement>("agent-token-ttl");
    expect(ttl.value).toBe("43200");
    await user.clear(ttl);
    await user.type(ttl, "15{Enter}");

    await waitFor(() =>
      expect(backend.patches).toEqual([{ oauth_access_token_ttl_minutes: 15 }]),
    );
  });

  it("refuses a lifetime under five minutes without sending it", async () => {
    const backend = backendFor(SETTINGS_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<AgentConnectionsCard />);

    const ttl = await screen.findByTestId<HTMLInputElement>("agent-token-ttl");
    await user.clear(ttl);
    await user.type(ttl, "4{Enter}");

    expect(
      await screen.findByText(translate("en", "agentConnections.ttl.refusal")),
    ).toBeTruthy();
    expect(backend.patches).toEqual([]);
  });

  it("is read-only to a seat without the update grant", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_READER).fetchMock);
    render(<AgentConnectionsCard />);

    const ttl = await screen.findByTestId<HTMLInputElement>("agent-token-ttl");
    expect(ttl.disabled).toBe(true);
    expect(
      screen.getByText(translate("en", "agentConnections.adminOnly")),
    ).toBeTruthy();
  });

  // The page opens on a sign-in grant a custom role can hold alone; the card
  // then withholds itself rather than drawing a refusal.
  it("draws nothing and asks nothing of a seat that cannot read the settings", async () => {
    const backend = backendFor({});
    vi.stubGlobal("fetch", backend.fetchMock);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { container } = render(<AgentConnectionsCard />, client);

    // Settled, not merely asked: while the grants are still loading every
    // grant reads as denied, so an earlier check would pass for the wrong reason.
    await waitFor(() =>
      expect(client.getQueryState(["me"])?.status).toBe("success"),
    );
    // And the render that answer causes, with its effects, has committed.
    await act(async () => {});
    expect(container.textContent).toBe("");
    expect(
      backend.fetchMock.mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes(
          "/installation/settings",
        ),
      ),
    ).toBe(false);
  });
});
