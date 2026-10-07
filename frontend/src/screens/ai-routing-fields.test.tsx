/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ProviderSettingsForm } from "./ai-provider-settings";
import { AdapterFields } from "./ai-routing-fields";

// A provider's host is set on the provider, once, so a lane's fields name the
// provider and the model and nothing about where the provider is — except the
// embeddings lane, which may sit on a server of its own.

type ProviderSettings = components["schemas"]["AiProviderSettings"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubModels() {
  // The model list is asked of the vendor while the fields are open; an empty
  // answer leaves the fields under test alone on screen.
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ models: [] }))),
  );
}

function wrap(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function mountLane(
  provider: string,
  laneName: string,
  providerSettings?: ProviderSettings,
  onChange: (next: {
    provider: string;
    model: string;
    base_url?: string;
  }) => void = () => undefined,
) {
  stubModels();
  wrap(
    <AdapterFields
      label="Provider"
      lane={
        laneName === "embeddings"
          ? "embeddings"
          : laneName === "decisions"
            ? "decisions"
            : "chat"
      }
      laneName={laneName}
      binding={{ provider, model: "m" }}
      catalogue={[]}
      providerSettings={providerSettings}
      disabled={false}
      onChange={onChange}
    />,
  );
}

describe("a lane's fields", () => {
  it("ask a tier for no host: it is the provider's", () => {
    mountLane("openai_compatible", "premium", {
      base_url: "https://openrouter.ai/api",
    });
    expect(screen.queryByLabelText("Host")).toBeNull();
    expect(screen.queryByText(/set its host/i)).toBeNull();
  });

  it("say where to set the host when the provider has none", () => {
    mountLane("openai_compatible", "premium", {});
    expect(screen.getByText(/set its host/i)).toBeInTheDocument();
  });

  it("raise no missing host for an embeddings lane on a server of its own", () => {
    stubModels();
    wrap(
      <AdapterFields
        label="Provider"
        lane="embeddings"
        laneName="embeddings"
        binding={{
          provider: "openai_compatible",
          model: "m",
          base_url: "http://vllm.internal:8000",
        }}
        catalogue={[]}
        providerSettings={{}}
        disabled={false}
        onChange={() => undefined}
      />,
    );
    expect(screen.queryByText(/set its host/i)).toBeNull();
  });

  it("let the embeddings lane name a server of its own", async () => {
    const changes: Array<{ base_url?: string }> = [];
    mountLane("vllm", "embeddings", undefined, (next) => changes.push(next));
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("Embeddings server"), "h");
    expect(changes.at(-1)?.base_url).toBe("h");
  });
});

// The Host field on the provider's sheet (its Other service) says what THIS
// adapter does with the
// address it is given. A chat broker, OpenRouter's decisions endpoint and a
// local decision server each append a different path, and only one of them has
// a default — so one sentence copied across all three tells two of them the
// wrong thing.
function mountSheet(provider: string) {
  stubModels();
  wrap(
    <ProviderSettingsForm
      provider={provider}
      // A host no known service has, so the sheet opens on Other and asks.
      routing={{
        profile: "cloud_frontier",
        tiers: {},
        embeddings: { provider: "gemini", model: "e" },
        providers: { [provider]: { base_url: "https://custom.example" } },
      }}
      canManage
    />,
  );
}

describe("the Host field's help", () => {
  it("tells a chat broker that /v1 is added and a host is required", () => {
    mountSheet("openai_compatible");
    expect(screen.getByLabelText("Host")).toHaveAccessibleDescription(
      /\/v1 is added.*Required/,
    );
  });

  // Any server on the Jev wire has no address of its own, so its endpoint is
  // required, and it is the full URL: nothing is appended to it.
  it("asks jev_compatible for its full endpoint and shows the shape it takes", () => {
    mountSheet("jev_compatible");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/used as written.*Required/);
    expect(host).toHaveAttribute(
      "placeholder",
      "https://openrouter.ai/api/alpha/decisions",
    );
    expect(host).not.toHaveAccessibleDescription(/is added/);
  });

  // TypeSafe's own API has a default endpoint, so its host is optional.
  it("offers jev its host, with the official endpoint it falls back to", () => {
    mountSheet("jev");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/Blank uses/);
    expect(host).toHaveAttribute(
      "placeholder",
      "https://api.typesafe.ai/v1/systemone",
    );
  });
});
