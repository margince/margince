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
// enforced EU profile would, and asks the chosen location whether it serves
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
  ],
};

const VERTEX_ROUTING = {
  profile: "eu_resident",
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
        return jsonResponse(vertexModels(question));
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

async function openLane(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
) {
  const lane = await screen.findByTestId(testId);
  await user.click(within(lane).getByRole("button", { name: /change/i }));
  return lane;
}

async function save(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: /preview effects/i }));
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: /save routing/i }),
    ).not.toBeDisabled(),
  );
  await user.click(screen.getByRole("button", { name: /save routing/i }));
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

describe("a gemini_vertex lane", () => {
  it("lists locations grouped EU, US, Other, Global, refusing the non-resident ones under eu_resident", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor().fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    await user.click(
      await within(lane).findByRole("combobox", { name: "Location" }),
    );
    const options = within(screen.getByRole("listbox")).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual([
      "EU residentEU · EU (multi-region) (eu)",
      "EU residentEU · Netherlands (europe-west4)",
      "Not residentUS · US (multi-region) (us) — outside EU data residency",
      "Not residentOther · London (europe-west2) — outside EU data residency",
      "Not residentGlobal · Global (global) — outside EU data residency",
    ]);
    const london = within(screen.getByRole("listbox")).getByRole("option", {
      name: /London/,
    });
    expect(london).toHaveAttribute("aria-disabled", "true");
    expect(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: /Netherlands/,
      }),
    ).not.toHaveAttribute("aria-disabled", "true");
  });

  it("offers every location under a profile that does not enforce residency", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({ routing: { ...VERTEX_ROUTING, profile: "eu_hosted" } })
        .fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
    await user.click(
      await within(lane).findByRole("combobox", { name: "Location" }),
    );
    const london = within(screen.getByRole("listbox")).getByRole("option", {
      name: "Other · London (europe-west2)",
    });
    expect(london).not.toHaveAttribute("aria-disabled", "true");
  });

  it("says it is asking while the locations are loading, and still shows the stored one", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor({ locations: () => new Promise<Response>(() => {}) })
        .fetchMock,
    );
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
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

    const lane = await openLane(user, "ai-routing-tier-premium");
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
      await within(lane).findByText(/couldn't verify this model in eu/i),
    ).toBeInTheDocument();
    expect(within(lane).queryByText(/not served/i)).toBeNull();
  });

  it("clears a model the new location does not serve, and saves the new location", async () => {
    const user = userEvent.setup();
    const backend = backendFor({
      vertexModels: ({ location, model }) =>
        location === "europe-west4" && model === "gemini-3.5-flash"
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
    await pickOption(
      user,
      await within(lane).findByRole("combobox", { name: "Location" }),
      "EU · Netherlands (europe-west4)",
    );
    expect(
      await within(lane).findByText(
        "gemini-3.5-flash is not served in europe-west4, so the field was cleared.",
      ),
    ).toBeInTheDocument();
    expect(within(lane).getByRole("combobox", { name: "Model" })).toHaveValue(
      "",
    );

    await pickSuggestion(
      user,
      within(lane).getByRole("combobox", { name: "Model" }),
      /^gemini-4\.0-flash/,
    );
    await save(user);
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.premium).toEqual({
      provider: "gemini_vertex",
      model: "gemini-4.0-flash",
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
              models: [{ id: "gemini-3.1-flash-lite", lane: "chat" }],
            }
          : LISTED_AT_EU,
    });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiRoutingCard />);

    const lane = await openLane(user, "ai-routing-tier-premium");
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
        name: /^gemini-3\.1-flash-lite/,
      }),
    ).toBeInTheDocument();
    expect(
      within(listbox).queryByRole("option", { name: /^gemini-4\.0-flash/ }),
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
    expect(
      await within(lane).findByRole("combobox", { name: "Location" }),
    ).toHaveTextContent("europe-west4");
    await save(user);
    await waitFor(() => expect(backend.getCapturedPut()).not.toBeNull());
    expect(backend.getCapturedPut()?.tiers.cheap_cloud.location).toBe(
      "europe-west4",
    );
  });
});

describe("withProvider", () => {
  it("drops the location leaving Vertex and the host arriving at it", () => {
    const vertex = withProvider(
      { provider: "openai_compatible", model: "m", base_url: "https://x" },
      "gemini_vertex",
      "eu",
    );
    expect(JSON.parse(JSON.stringify(vertex))).toEqual({
      provider: "gemini_vertex",
      model: "m",
      location: "eu",
    });
    const back = withProvider(vertex, "gemini", "eu");
    expect(JSON.parse(JSON.stringify(back))).toEqual({
      provider: "gemini",
      model: "m",
    });
  });
});
