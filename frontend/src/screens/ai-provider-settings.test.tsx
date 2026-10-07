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
import { serviceOf } from "./ai-provider-settings";

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
  routingRead: () => Promise<Response> = async () => jsonResponse(routing),
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
              usable: true,
              optional: false,
            },
            {
              provider: "gemini_vertex",
              configured: true,
              env_var: "GEMINI_VERTEX_SA_JSON",
              usable: true,
              optional: false,
              credential_kind: "service_account",
            },
            {
              provider: "jev_compatible",
              configured: false,
              env_var: "JEV_COMPATIBLE_API_KEY",
              usable: true,
              optional: true,
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
        return routingRead();
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

async function pickService(
  user: ReturnType<typeof userEvent.setup>,
  sheet: HTMLElement,
  name: string,
) {
  await user.click(
    await within(sheet).findByRole("combobox", { name: "Service" }),
  );
  await user.click(await screen.findByRole("option", { name }));
}

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
  // A provider reads by its product name; the variable its key may arrive in
  // stays on the sheet, where the key is managed.
  it("names each provider the way its vendor does", async () => {
    backend(routingWith({}));
    render(<AiProviderKeysCard />);

    const row = await screen.findByTestId("ai-provider-row-openai_compatible");
    expect(row).toHaveTextContent("OpenAI-compatible");
    expect(within(row).queryByText("OPENAI_COMPATIBLE_API_KEY")).toBeNull();
    expect(
      screen.getByTestId("ai-provider-row-gemini_vertex"),
    ).toHaveTextContent("Gemini on Vertex AI");
  });

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
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts).toEqual([
        {
          provider: "openai_compatible",
          body: { base_url: "https://gateway.example" },
        },
      ]),
    );
  });

  // The form starts from what is stored; drawn before the routing read lands
  // it would start empty, and Save would then remove the stored entry.
  it("waits for the stored settings before drawing the form", async () => {
    let release: (r: Response) => void = () => undefined;
    const routing = routingWith({
      openai_compatible: { base_url: "https://old.example" },
    });
    backend(
      routing,
      () => jsonResponse(routing),
      () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    expect(within(sheet).queryByLabelText("Host")).toBeNull();
    release(jsonResponse(routing));
    expect(await within(sheet).findByLabelText("Host")).toHaveValue(
      "https://old.example",
    );
  });

  it("fills a known service's host when it is chosen", async () => {
    const puts = backend(routingWith({}));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await pickService(user, sheet, "Mistral");
    expect(
      within(sheet).getByText(/https:\/\/api\.mistral\.ai/),
    ).toBeInTheDocument();
    expect(within(sheet).queryByLabelText("Host")).toBeNull();
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts[0]?.body).toEqual({ base_url: "https://api.mistral.ai" }),
    );
  });

  it("opens on the service its stored host belongs to", async () => {
    backend(
      routingWith({
        openai_compatible: { base_url: "https://openrouter.ai/api/" },
      }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    expect(
      await within(sheet).findByRole("combobox", { name: "Service" }),
    ).toHaveTextContent("OpenRouter");
  });

  // OpenRouter's EU address keeps processing inside the EU; it needs a plan
  // OpenRouter sells, which the sheet says before Save rather than after.
  it("offers OpenRouter's EU address with what it requires", async () => {
    const puts = backend(routingWith({}));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await pickService(user, sheet, "OpenRouter (EU)");
    expect(
      within(sheet).getByText(/Business or Enterprise plan/),
    ).toBeInTheDocument();
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts[0]?.body.base_url).toBe("https://eu.openrouter.ai/api"),
    );
  });

  // The pins are no longer on this sheet; ones already stored are not wiped by
  // a save that never showed them.
  it("keeps stored pins it does not show", async () => {
    const puts = backend(
      routingWith({
        openai_compatible: {
          base_url: "https://openrouter.ai/api",
          upstream: { only: ["mistral/eu"] },
        },
      }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await within(sheet).findByRole("combobox", { name: "Service" });
    expect(within(sheet).queryByText("Only these hosts")).toBeNull();
    await user.click(
      within(sheet).getByRole("checkbox", { name: /Zero data retention/ }),
    );
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts[0]?.body).toEqual({
        base_url: "https://openrouter.ai/api",
        upstream: { zdr: true, only: ["mistral/eu"] },
      }),
    );
  });

  // Pins name OpenRouter's hosts; a service elsewhere cannot take them, so
  // moving off OpenRouter drops them rather than being refused for them.
  it("drops stored pins when the provider moves off OpenRouter", async () => {
    const puts = backend(
      routingWith({
        openai_compatible: {
          base_url: "https://openrouter.ai/api",
          upstream: { only: ["mistral/eu"] },
        },
      }),
    );
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    await pickService(user, sheet, "Mistral");
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts[0]?.body).toEqual({ base_url: "https://api.mistral.ai" }),
    );
  });

  // With no host stored the sheet states none: the reader chooses a service,
  // and only then can it be saved.
  it("asks for a service when the provider has no host yet", async () => {
    backend(routingWith({}));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "openai_compatible");
    expect(
      await within(sheet).findByRole("combobox", { name: "Service" }),
    ).toHaveTextContent("Choose a service");
    expect(within(sheet).queryByText(/Host: /)).toBeNull();
    expect(
      within(sheet).getByRole("button", { name: "Save connection" }),
    ).toBeDisabled();
  });

  it("offers OpenRouter's EU address for a decision server", async () => {
    const puts = backend(routingWith({}));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "jev_compatible");
    await pickService(user, sheet, "OpenRouter (EU)");
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    await waitFor(() =>
      expect(puts[0]?.body.base_url).toBe(
        "https://eu.openrouter.ai/api/alpha/decisions",
      ),
    );
  });

  it("sets a Vertex location on the provider", async () => {
    const puts = backend(routingWith({ gemini_vertex: { location: "eu" } }));
    const user = userEvent.setup();
    render(<AiProviderKeysCard />);

    const sheet = await openSheet(user, "gemini_vertex");
    expect(within(sheet).queryByLabelText("Host")).toBeNull();
    await user.click(await within(sheet).findByLabelText("Location"));
    await user.click(
      await screen.findByRole("option", { name: /europe-west4/ }),
    );
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

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
    await user.click(
      within(sheet).getByRole("button", { name: "Save connection" }),
    );

    expect(await within(sheet).findByText(/tier premium/)).toBeInTheDocument();
  });
});

describe("serviceOf", () => {
  it("takes a host carrying credentials or a query for no listed service", () => {
    expect(serviceOf("openai_compatible", "https://openrouter.ai/api")).toBe(
      "openrouter",
    );
    expect(
      serviceOf("openai_compatible", "https://team:secret@openrouter.ai/api"),
    ).toBe("other");
    expect(
      serviceOf("openai_compatible", "https://openrouter.ai/api?region=eu"),
    ).toBe("other");
  });
});
