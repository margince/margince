/** @vitest-environment jsdom */
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
import { pickOption } from "../design-system/select-testing";
import { type Locale, LocaleProvider } from "../i18n";
import { AiRoutingCard } from "./ai-routing";

// Settings → AI → Model routing: which vendor this installation's text is sent
// to. The server is the RBAC authority; this screen mirrors it by disabling
// (never hiding) the save for a reader who may not change it, so an operator
// can still SEE the binding they are asking somebody else to change.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Naming the grant rather than a role keeps the fixture honest about what the
// screen actually asks for.
const ROUTING_EDITOR: GrantSpec = { ai_routing: ["read", "update"] };
const ROUTING_READER: GrantSpec = { ai_routing: ["read"] };

const BOUND = {
  profile: "eu_hosted",
  tiers: {
    premium: { provider: "gemini", model: "gemini-3.5-flash" },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

function backendFor(allow: GrantSpec, routing: unknown = BOUND) {
  let stored = routing;
  let capturedPut: unknown = null;
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/ai/routing")) {
        if (req.method === "PUT") {
          capturedPut = await req.json();
          stored = capturedPut;
        }
        return jsonResponse(stored);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, getCapturedPut: () => capturedPut };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
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

describe("AiRoutingCard", () => {
  it("shows the bound model for each tier", async () => {
    vi.stubGlobal("fetch", backendFor(ROUTING_EDITOR).fetchMock);
    render(<AiRoutingCard />);

    expect(await screen.findByDisplayValue("gemini-3.5-flash")).toBeTruthy();
    expect(screen.getByDisplayValue("gemini-3.1-flash-lite")).toBeTruthy();
  });

  it("sends the WHOLE binding, so an untouched tier is not dropped", async () => {
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const model = await screen.findByDisplayValue("gemini-3.5-flash");
    await userEvent.clear(model);
    await userEvent.type(model, "gemini-3.1-pro-preview");
    await userEvent.click(
      screen.getByRole("button", { name: /save routing/i }),
    );

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    // PUT replaces the whole document, so the tier nobody touched has to travel
    // with the one that changed — sending only the edit would unbind the rest.
    expect(backend.getCapturedPut()).toEqual({
      profile: "eu_hosted",
      tiers: {
        premium: { provider: "gemini", model: "gemini-3.1-pro-preview" },
        cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
      },
      embeddings: { provider: "gemini", model: "gemini-embedding-001" },
    });
  });

  it("refuses the save to a reader who may not change it, and still shows the binding", async () => {
    vi.stubGlobal("fetch", backendFor(ROUTING_READER).fetchMock);
    render(<AiRoutingCard />);

    // Disabled, never hidden: somebody who cannot change the binding still
    // needs to see which vendor their installation's text goes to.
    expect(await screen.findByDisplayValue("gemini-3.5-flash")).toBeTruthy();
    // `disabled`, not `aria-disabled`: the design system reserves the second
    // for a write in flight, so a reader keeps focus on the control they just
    // pressed. A refusal is the first, and the reason travels with it.
    await waitFor(() =>
      expect(
        (
          screen.getByRole("button", {
            name: /save routing/i,
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(true),
    );
  });

  it("says an unbound installation is unbound rather than drawing an empty form", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ROUTING_EDITOR, {
        profile: "eu_hosted",
        tiers: {},
        embeddings: { provider: "", model: "" },
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    expect(await screen.findByText(/no models bound/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /save routing/i })).toBeNull();
  });

  // Cheapest first, most capable last — the ladder, not the alphabet.
  // Alphabetically `cheap_cloud` sits above `frontier` and `local_small` above
  // `premium`, an order that tells a reader nothing about what they are
  // choosing between.
  it("lists the tiers in ladder order, not alphabetically", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ROUTING_EDITOR, {
        profile: "eu_hosted",
        tiers: {
          frontier: { provider: "gemini", model: "f" },
          local_small: { provider: "gemini", model: "ls" },
          premium: { provider: "gemini", model: "p" },
          cheap_cloud: { provider: "gemini", model: "cc" },
        },
        embeddings: { provider: "gemini", model: "e" },
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    await screen.findByDisplayValue("ls");
    const shown = screen
      .getAllByTestId(/^ai-routing-tier-/)
      .map((el) => el.getAttribute("data-testid"));
    expect(shown).toEqual([
      "ai-routing-tier-local_small",
      "ai-routing-tier-cheap_cloud",
      "ai-routing-tier-premium",
      "ai-routing-tier-frontier",
    ]);
  });

  // The embed lane was missing from this form entirely, so a reader could
  // re-point every chat tier and go on sending their retrieval to the vendor
  // they had just moved away from, with nothing on screen saying so.
  it("lets the embedding lane be re-pointed, and sends it", async () => {
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const model = await screen.findByDisplayValue("gemini-embedding-001");
    await userEvent.clear(model);
    await userEvent.type(model, "gemini-embedding-002");
    await userEvent.click(
      screen.getByRole("button", { name: /save routing/i }),
    );

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(
      (backend.getCapturedPut() as { embeddings: { model: string } }).embeddings
        .model,
    ).toBe("gemini-embedding-002");
  });

  // openai_compatible has no default host and the server refuses a binding
  // without one. With no field for it, choosing that adapter produced a write
  // the running role could never adopt: saved cleanly, then declined at the
  // rebind, leaving the OLD models serving with the reason only in a log.
  it("asks for a host when the adapter has no default, and only then", async () => {
    // An instance rather than the default export: pickOption drives a portalled
    // listbox and needs a session that keeps pointer state across the open.
    const user = userEvent.setup();
    const backend = backendFor(ROUTING_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);
    await screen.findByDisplayValue("gemini-3.5-flash");

    // A native vendor addresses its own API, so no host is asked for.
    expect(screen.queryByLabelText("Host")).toBeNull();

    const tier = screen.getByTestId("ai-routing-tier-premium");
    await pickOption(
      user,
      within(tier).getByRole("combobox"),
      "openai_compatible",
    );
    const host = await within(tier).findByLabelText("Host");
    await user.type(host, "https://openrouter.ai/api");
    await userEvent.click(
      screen.getByRole("button", { name: /save routing/i }),
    );

    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    const sent = backend.getCapturedPut() as {
      tiers: Record<string, { provider: string; base_url?: string }>;
    };
    expect(sent.tiers.premium.provider).toBe("openai_compatible");
    expect(sent.tiers.premium.base_url).toBe("https://openrouter.ai/api");
  });
});
