/** @vitest-environment happy-dom */
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
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";

// A provider's host, its OpenRouter pins and its Vertex location are set on
// the provider's sheet, once, for every lane that binds it.

type Routing = components["schemas"]["AiRouting"];
type ProviderSettings = components["schemas"]["AiProviderSettings"];

const EDITOR: GrantSpec = { ai_routing: ["read", "update"] };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function routingWith(providers: Routing["providers"]): Routing {
  return {
    profile: "cloud_frontier",
    tiers: { premium: { provider: "openai_compatible", model: "m" } },
    embeddings: { provider: "gemini_vertex", model: "gemini-embedding-001" },
    providers,
  };
}

function backend(
  routing: Routing,
  answer: (body: ProviderSettings) => Response = () =>
    jsonResponse(routing, 200),
) {
  const puts: Array<{ provider: string; body: ProviderSettings }> = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: EDITOR }));
      }
      if (req.url.includes("/ai/provider-settings/")) {
        const body = (await req.json()) as ProviderSettings;
        puts.push({ provider: req.url.split("/").pop() ?? "", body });
        return answer(body);
      }
      if (req.url.includes("/ai/provider-keys")) {
        return jsonResponse({
          providers: [
            {
              provider: "openai_compatible",
              configured: true,
              env_var: "OPENAI_COMPATIBLE_API_KEY",
              optional: false,
            },
            {
              provider: "gemini_vertex",
              configured: true,
              env_var: "GEMINI_VERTEX_SA_JSON",
              optional: false,
              credential_kind: "service_account",
            },
          ],
        });
      }
      if (req.url.includes("/ai/provider-locations")) {
        return jsonResponse({
          provider: "gemini_vertex",
          locations: [
            { id: "eu", jurisdiction: "eu", resident: true },
            { id: "europe-west4", jurisdiction: "eu", resident: true },
          ],
        });
      }
      if (req.url.includes("/ai/routing")) {
        return jsonResponse(routing);
      }
      if (req.url.includes("/ai-model-rates")) {
        return jsonResponse({}, 404);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return puts;
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

async function openSheet(
  user: ReturnType<typeof userEvent.setup>,
  provider: string,
) {
  const row = await screen.findByTestId(`ai-provider-row-${provider}`);
  await user.click(within(row).getByRole("button"));
  return screen.findByRole("dialog");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a provider's settings on its sheet", () => {
  it("shows the stored host and saves a new one through the provider", async () => {
    const puts = backend(
      routingWith({ openai_compatible: { base_url: "https://old.example" } }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    const host = await within(sheet).findByLabelText("Host");
    expect(host).toHaveValue("https://old.example");
    await user.clear(host);
    await user.type(host, "https://gateway.example");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(puts).toEqual([
        {
          provider: "openai_compatible",
          body: { base_url: "https://gateway.example" },
        },
      ]),
    );
  });

  it("fills OpenRouter's host from the preset", async () => {
    const puts = backend(routingWith({}));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await user.click(
      await within(sheet).findByRole("button", { name: "Preset: OpenRouter" }),
    );
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(puts[0]?.body.base_url).toBe("https://openrouter.ai/api"),
    );
  });

  it("offers the upstream pins only for an OpenRouter host", async () => {
    backend(
      routingWith({ openai_compatible: { base_url: "https://gateway.example" } }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await within(sheet).findByLabelText("Host");
    expect(
      within(sheet).queryByRole("group", { name: "OpenRouter hosts" }),
    ).toBeNull();
  });

  it("pins the hosts listed, and sends no upstream when none are", async () => {
    const puts = backend(
      routingWith({
        openai_compatible: {
          base_url: "https://openrouter.ai/api",
          upstream: { ignore: ["deepinfra"] },
        },
      }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    const hosts = await within(sheet).findByRole("group", {
      name: "OpenRouter hosts",
    });
    await user.type(
      within(hosts).getByLabelText("Only these hosts"),
      "mistral/eu{Enter}",
    );
    await user.click(within(sheet).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toEqual({
      base_url: "https://openrouter.ai/api",
      upstream: { only: ["mistral/eu"], ignore: ["deepinfra"] },
    });

    await user.click(
      within(hosts).getByRole("button", { name: "Remove mistral/eu" }),
    );
    await user.click(
      within(hosts).getByRole("button", { name: "Remove deepinfra" }),
    );
    await user.click(within(sheet).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(2));
    expect(puts[1]?.body).toEqual({ base_url: "https://openrouter.ai/api" });
  });

  it("sets a Vertex location on the provider", async () => {
    const puts = backend(routingWith({ gemini_vertex: { location: "eu" } }));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "gemini_vertex");
    expect(within(sheet).queryByLabelText("Host")).toBeNull();
    await user.click(await within(sheet).findByLabelText("Location"));
    await user.click(await screen.findByRole("option", { name: /europe-west4/ }));
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(puts).toEqual([
        { provider: "gemini_vertex", body: { location: "europe-west4" } },
      ]),
    );
  });

  it("says which lanes still need a host it would clear", async () => {
    backend(
      routingWith({ openai_compatible: { base_url: "https://old.example" } }),
      () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            status: 422,
            code: "no_host",
            detail:
              "openai_compatible is bound by tier premium; give it a host or rebind those lanes first",
          },
          422,
        ),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await user.clear(await within(sheet).findByLabelText("Host"));
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    expect(await within(sheet).findByText(/tier premium/)).toBeInTheDocument();
  });
});
