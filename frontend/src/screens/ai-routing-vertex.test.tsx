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
import { type GrantSpec, meFixture } from "../app/mefixture";
import { pickOption, pickSuggestion } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { AiRoutingCard } from "./ai-routing";
import { withProvider } from "./ai-routing-fields";

// A Gemini-on-Vertex lane is bound by LOCATION, which is where Google processes
// the call: the field lists locations by jurisdiction, refuses the ones the
// eu_hosted profile would, and asks the chosen location whether it serves
// the chosen model before the save has to.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ETag: '"routing-v1"' },
  });
}

const ROUTING_EDITOR: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_budget: ["read"],
};

const LOCATIONS = {
  provider: "gemini_vertex",
  locations: [
    {
      id: "us",
      display_name: "US (multi-region)",
      jurisdiction: "us",
      resident: false,
    },
    {
      id: "europe-west2",
      display_name: "London",
      jurisdiction: "other",
      resident: false,
    },
    {
      id: "global",
      display_name: "Global",
      jurisdiction: "global",
      resident: false,
    },
    {
      id: "europe-west4",
      display_name: "Netherlands",
      jurisdiction: "eu",
      resident: true,
    },
    {
      id: "eu",
      display_name: "EU (multi-region)",
      jurisdiction: "eu",
      resident: true,
    },
    // Google names no region, only the multi-regions.
    {
      id: "europe-west1",
      display_name: "",
      jurisdiction: "eu",
      resident: true,
    },
  ],
};

const VERTEX_ROUTING = {
  profile: "eu_hosted",
  tiers: {
    premium: {
      provider: "gemini_vertex",
      model: "gemini-3.5-flash",
      location: "eu",
    },
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: {
    provider: "gemini_vertex",
    model: "gemini-embedding-001",
    location: "eu",
  },
};

const LISTED_AT_EU = {
  provider: "gemini_vertex",
  models: [
    { id: "gemini-4.0-flash", lane: "chat" },
    { id: "gemini-3.5-flash", lane: "chat" },
  ],
};

const EMBEDDERS_AT_EU = {
  provider: "gemini_vertex",
  models: [
    { id: "gemini-embedding-002", lane: "embeddings" },
    { id: "gemini-embedding-001", lane: "embeddings" },
  ],
};

type CapturedBinding = { provider: string; model: string; location?: string };
type CapturedRouting = {
  profile: string;
  tiers: Record<string, CapturedBinding>;
  embeddings: CapturedBinding;
};

type Answer = Readonly<{ location: string | null; model: string | null }>;

function backendFor({
  routing = VERTEX_ROUTING,
  locations = () => Promise.resolve(jsonResponse(LOCATIONS)),
  vertexModels = () => LISTED_AT_EU,
}: {
  routing?: unknown;
  locations?: () => Promise<Response>;
  vertexModels?: (asked: Answer) => unknown;
} = {}) {
  const asked: Answer[] = [];
  let capturedPut: CapturedRouting | null = null;
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(req.url);
      if (url.pathname.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: ROUTING_EDITOR }));
      }
      if (url.pathname.endsWith("/ai/provider-locations/gemini_vertex")) {
        return locations();
      }
      if (url.pathname.endsWith("/ai/available-models/gemini_vertex")) {
        const question = {
          location: url.searchParams.get("location"),
          model: url.searchParams.get("model"),
        };
        asked.push(question);
        const answer = vertexModels(question);
        return answer instanceof Promise ? answer : jsonResponse(answer);
      }
      if (url.pathname.includes("/ai/available-models/")) {
        return jsonResponse({ provider: "gemini", models: [] });
      }
      if (url.pathname.endsWith("/ai/provider-keys")) {
        return jsonResponse({ providers: [] });
      }
      if (url.pathname.endsWith("/ai-model-rates")) {
        return jsonResponse({ data: [] });
      }
      if (url.pathname.endsWith("/ai/routing/preview")) {
        return jsonResponse({
          current_version: "routing-v1",
          features: [],
          unused_tiers: [],
        });
      }
      if (url.pathname.endsWith("/ai/routing")) {
        if (req.method === "PUT") {
          capturedPut = await req.json();
        }
        return jsonResponse(capturedPut ?? routing);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, asked, getCapturedPut: () => capturedPut };
}

// Opens one lane's editor and hands back the dialog that owns its binding.
async function openLane(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
) {
  const lane = await screen.findByTestId(testId);
  await user.click(within(lane).getByRole("button", { name: /^edit\b/i }));
  return screen.findByRole("dialog");
}

async function save(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", {
      name: /save binding/i,
    }),
  );
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A tier is served at its provider's location, set on the provider's sheet;
// the embeddings lane may name one of its own, so the location field — and
// everything it asks Google — is exercised there.
describe("a gemini_vertex lane", () => {
  it("lists locations grouped EU, US, Other, Global and refuses none of them", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor().fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-embeddings");
    await user.click(
      await within(lane).findByRole("combobox", { name: "Location" }),
    );
    const options = within(screen.getByRole("listbox")).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual([
      "EU residentEU · EU (multi-region) (eu)",
      "EU residentEU · europe-west1",
      "EU residentEU · Netherlands (europe-west4)",
      "Not residentUS · US (multi-region) (us)",
      "Not residentOther · London (europe-west2)",
      "Not residentGlobal · Global (global)",
    ]);
    for (const option of options) {
      expect(option).not.toHaveAttribute("aria-disabled", "true");
    }
  });

  it("says it is asking while the locations are loading, and still shows the stored one", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({ locations: () => new Promise<Response>(() => {}) })
        .fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-embeddings");
    expect(
      await within(lane).findByText(/asking google which locations/i),
    ).toBeInTheDocument();
    expect(
      within(lane).getByRole("combobox", { name: "Location" }),
    ).toHaveTextContent("eu");
  });

  it("points at the provider keys when no service-account key is held", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({
        locations: () =>
          Promise.resolve(
            jsonResponse({
              provider: "gemini_vertex",
              locations: [],
              unavailable: "no_key",
            }),
          ),
        vertexModels: () => ({
          provider: "gemini_vertex",
          models: [],
          unavailable: "no_key",
        }),
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-embeddings");
    const notes = await within(lane).findAllByText(
      /add it under model provider keys/i,
    );
    // Both fields say it: the location list and the model list are each
    // blocked on the same missing key.
    expect(notes).toHaveLength(2);
  });

  it("says when the chosen location lists no models", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({
        vertexModels: () => ({ provider: "gemini_vertex", models: [] }),
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    expect(
      await within(lane).findByText(/eu serves none of the models/i),
    ).toBeInTheDocument();
  });

  it("flags a picked model the location does not serve", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      vertexModels: ({ model }) =>
        model === "gemini-4.0-flash"
          ? {
              provider: "gemini_vertex",
              models: [],
              unavailable: "no_endpoint",
            }
          : LISTED_AT_EU,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    await pickSuggestion(
      user,
      await within(lane).findByRole("combobox", { name: "Model" }),
      /^gemini-4\.0-flash/,
    );
    expect(
      await within(lane).findByText(/not served in eu\./i),
    ).toBeInTheDocument();
    expect(backend.asked).toContainEqual({
      location: "eu",
      model: "gemini-4.0-flash",
    });
  });

  it("says a probe that could not run could not verify, rather than calling the model unserved", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({
        vertexModels: ({ model }) =>
          model
            ? {
                provider: "gemini_vertex",
                models: [],
                unavailable: "unreachable",
              }
            : LISTED_AT_EU,
      }).fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    await pickSuggestion(
      user,
      await within(lane).findByRole("combobox", { name: "Model" }),
      /^gemini-4\.0-flash/,
    );
    expect(
      await within(lane).findByText(/could not verify this model in eu/i),
    ).toBeInTheDocument();
    expect(within(lane).queryByText(/not served/i)).toBeNull();
  });

  it("asks again about a model whose probe went unanswered", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      vertexModels: ({ model }) =>
        model
          ? {
              provider: "gemini_vertex",
              models: [],
              unavailable: "unreachable",
            }
          : LISTED_AT_EU,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    const box = await within(lane).findByRole("combobox", { name: "Model" });
    await pickSuggestion(user, box, /^gemini-4\.0-flash/);
    await within(lane).findByText(/could not verify this model in eu/i);
    // Away and back: the same question, which must reach Google again.
    for (const model of [/^gemini-3\.5-flash/, /^gemini-4\.0-flash/]) {
      await user.clear(box);
      await user.click(
        within(await screen.findByRole("listbox")).getByRole("option", {
          name: model,
        }),
      );
    }
    await waitFor(() =>
      expect(
        backend.asked.filter((q) => q.model === "gemini-4.0-flash"),
      ).toHaveLength(2),
    );
  });

  it("clears a model the new location does not serve, and saves the new location", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      vertexModels: ({ location, model }) =>
        location === "europe-west4" && model === "gemini-embedding-001"
          ? {
              provider: "gemini_vertex",
              models: [],
              unavailable: "no_endpoint",
            }
          : EMBEDDERS_AT_EU,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-embeddings");
    await pickOption(
      user,
      await within(lane).findByRole("combobox", { name: "Location" }),
      "EU · Netherlands (europe-west4)",
    );
    expect(
      await within(lane).findByText(
        "gemini-embedding-001 is not served in europe-west4, so the field was cleared.",
      ),
    ).toBeInTheDocument();
    expect(within(lane).getByRole("combobox", { name: "Model" })).toHaveValue(
      "",
    );

    await pickSuggestion(
      user,
      within(lane).getByRole("combobox", { name: "Model" }),
      /^gemini-embedding-002/,
    );
    await save(user);
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.embeddings).toMatchObject({
      provider: "gemini_vertex",
      model: "gemini-embedding-002",
      location: "europe-west4",
    });
  });

  it("asks the new location for its models when the location changes", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      vertexModels: ({ location }) =>
        location === "europe-west4"
          ? {
              provider: "gemini_vertex",
              models: [{ id: "text-embedding-005", lane: "embeddings" }],
            }
          : EMBEDDERS_AT_EU,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-embeddings");
    await pickOption(
      user,
      await within(lane).findByRole("combobox", { name: "Location" }),
      "EU · Netherlands (europe-west4)",
    );
    // Emptied first: the list is filtered by what the box holds.
    await user.clear(within(lane).getByRole("combobox", { name: "Model" }));
    const listbox = await screen.findByRole("listbox");
    expect(
      await within(listbox).findByRole("option", {
        name: /^text-embedding-005/,
      }),
    ).toBeInTheDocument();
    expect(
      within(listbox).queryByRole("option", { name: /^gemini-embedding-002/ }),
    ).toBeNull();
  });

  it("starts a lane re-pointed at Vertex at the saved Vertex location, and sends none for other providers", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      routing: {
        ...VERTEX_ROUTING,
        tiers: {
          ...VERTEX_ROUTING.tiers,
          premium: {
            ...VERTEX_ROUTING.tiers.premium,
            location: "europe-west4",
          },
        },
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-cheap_cloud");
    await pickOption(
      user,
      within(lane).getByRole("combobox", { name: "Provider" }),
      "gemini_vertex",
    );
    await user.type(
      within(lane).getByRole("combobox", { name: "Model" }),
      "gemini-3.5-flash",
    );
    // The tier names no location of its own: it is the provider's.
    expect(
      within(lane).queryByRole("combobox", { name: "Location" }),
    ).toBeNull();
    await save(user);
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.cheap_cloud.location).toBe(
      "europe-west4",
    );
  });
});

describe("a Vertex model list still being asked", () => {
  // Every model is asked of the location, which takes a moment; until it
  // answers, the price sheet's models are not offered in its place, because
  // most of them that location does not serve.
  it("offers nothing and says it is asking until the location answers", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({ vertexModels: () => new Promise<Response>(() => {}) })
        .fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    const box = within(lane).getByRole("combobox", { name: "Model" });
    await user.clear(box);
    expect(
      await within(lane).findByText(/Asking Google which models eu serves/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});

describe("a tier newly pointed at Vertex", () => {
  // The provider's location is where every tier on it is served, so a tier
  // starts there rather than at the embeddings lane's own location.
  it("starts at the provider's location, not the embeddings lane's", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      routing: {
        ...VERTEX_ROUTING,
        tiers: {
          cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
        },
        embeddings: { ...VERTEX_ROUTING.embeddings, location: "europe-west1" },
        providers: { gemini_vertex: { location: "europe-west4" } },
      },
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-cheap_cloud");
    await pickOption(
      user,
      within(lane).getByRole("combobox", { name: "Provider" }),
      "gemini_vertex",
    );
    await user.type(
      within(lane).getByRole("combobox", { name: "Model" }),
      "gemini-3.5-flash",
    );
    await save(user);
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.cheap_cloud.location).toBe(
      "europe-west4",
    );
  });
});

describe("withProvider", () => {
  // A model id names a model on ONE vendor: carried onto another it is a model
  // that vendor does not serve, so a provider change empties it.
  it("empties the model and drops what belongs to the old provider", () => {
    const vertex = withProvider(
      {
        provider: "openai_compatible",
        model: "openai/gpt-oss-120b",
        base_url: "https://x",
      },
      "gemini_vertex",
      "eu",
    );
    expect(JSON.parse(JSON.stringify(vertex))).toEqual({
      provider: "gemini_vertex",
      model: "",
      location: "eu",
    });
    const back = withProvider(
      { ...vertex, model: "gemini-3.5-flash" },
      "gemini",
      "eu",
    );
    expect(JSON.parse(JSON.stringify(back))).toEqual({
      provider: "gemini",
      model: "",
    });
  });

  it("keeps the model when the provider does not change", () => {
    const same = withProvider(
      { provider: "gemini", model: "gemini-3.5-flash" },
      "gemini",
      "eu",
    );
    expect(same.model).toBe("gemini-3.5-flash");
  });
});
