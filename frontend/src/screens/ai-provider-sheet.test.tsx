/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const KEYS = {
  providers: [
    {
      provider: "gemini",
      configured: true,
      env_var: "GEMINI_API_KEY",
      optional: false,
    },
    {
      provider: "anthropic",
      configured: true,
      env_var: "ANTHROPIC_API_KEY",
      optional: false,
    },
    {
      provider: "openai",
      configured: false,
      env_var: "OPENAI_API_KEY",
      optional: false,
    },
    {
      provider: "jev",
      configured: false,
      env_var: "TYPESAFE_API_KEY",
      optional: false,
    },
  ],
};

// gemini is bound (Active), anthropic holds a key nothing uses (Ready), jev is
// bound with no key (Needs key), openai is neither (Not active).
const ROUTING = {
  tiers: {
    cheap_cloud: { provider: "gemini", model: "gemini-3.5-flash" },
    premium: { provider: "jev", model: "jev-1" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

const GEMINI_ROW = {
  provider: "gemini",
  model_id: "gemini-3.5-flash",
  lane: "chat",
  input_per_mtok: "0.3",
  output_per_mtok: "2.5",
  cache_read_per_mtok: "0.03",
  cache_write_per_mtok: "0",
  effective_date: "2026-08-01",
};

const WRITER: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_model_rate: ["read", "create", "update"],
};

type Posted = { url: string; body: unknown };

function backend(allow: GrantSpec, posts: Posted[]) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const path = new URL(req.url).pathname;
    if (path.endsWith("/me")) return jsonResponse(meFixture({ allow }));
    if (path.endsWith("/ai/provider-keys")) return jsonResponse(KEYS);
    if (path.endsWith("/ai/routing")) return jsonResponse(ROUTING);
    if (path.includes("/ai/available-models/")) {
      return jsonResponse({
        provider: "gemini",
        models: [{ id: "gemini-4-pro" }],
      });
    }
    if (path.endsWith("/ai-model-rates/refresh")) {
      return jsonResponse({
        providers: [
          {
            provider: "gemini",
            outcome: "not_available",
            updated: 0,
            unchanged: 0,
            models: [],
            unlisted: [],
          },
        ],
      });
    }
    if (path.endsWith("/ai-model-rates") && req.method === "POST") {
      posts.push({ url: req.url, body: await req.json() });
      return jsonResponse(GEMINI_ROW, 201);
    }
    if (path.endsWith("/ai-model-rates")) {
      return jsonResponse({
        data: [
          GEMINI_ROW,
          { ...GEMINI_ROW, provider: "anthropic", model_id: "claude-x" },
        ],
      });
    }
    return jsonResponse({}, 404);
  });
}

function mount(allow: GrantSpec = WRITER) {
  const posts: Posted[] = [];
  vi.stubGlobal("fetch", backend(allow, posts));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <LocaleProvider>
        <AiProviderKeysCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { posts };
}

async function open(
  user: ReturnType<typeof userEvent.setup>,
  provider: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Manage ${provider}` }),
  );
  return screen.findByRole("dialog", { name: provider });
}

describe("the Providers list", () => {
  it("reads each vendor as active, ready, needing a key or not active", async () => {
    mount();
    const state = async (provider: string) =>
      (await screen.findByTestId(`ai-provider-row-${provider}`)).textContent;
    await vi.waitFor(async () =>
      expect(await state("gemini")).toContain("cheap_cloud, embeddings"),
    );
    expect(await state("gemini")).toContain("Active");
    expect(await state("anthropic")).toContain("Ready");
    expect(await state("anthropic")).toContain("Not used");
    expect(await state("jev")).toContain("Needs key");
    expect(await state("openai")).toContain("Not active");
  });
});

describe("a provider's sheet", () => {
  it("links the vendor's own price list and lists only its prices", async () => {
    const user = userEvent.setup();
    mount();
    const sheet = await open(user, "gemini");
    const link = await within(sheet).findByRole("link", {
      name: /price list/i,
    });
    expect(link.getAttribute("href")).toBe(
      "https://ai.google.dev/gemini-api/docs/pricing",
    );
    const table = within(sheet).getByRole("table");
    expect(
      within(table).getAllByText("gemini-3.5-flash").length,
    ).toBeGreaterThan(0);
    expect(within(table).queryByText("claude-x")).toBeNull();
  });

  it("swaps to the form inside the sheet to add a price, then back", async () => {
    const user = userEvent.setup();
    const { posts } = mount();
    const sheet = await open(user, "gemini");
    await user.click(
      await within(sheet).findByRole("button", { name: "Add price" }),
    );
    expect(within(sheet).queryByRole("table")).toBeNull();

    await user.type(within(sheet).getByLabelText("Model"), "gemini-4-pro");
    await user.type(within(sheet).getByLabelText("Input $/M"), "2");
    await user.type(within(sheet).getByLabelText("Output $/M"), "12");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    await vi.waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]?.body).toMatchObject({
      provider: "gemini",
      model_id: "gemini-4-pro",
      lane: "chat",
      input_per_mtok: "2",
      output_per_mtok: "12",
    });
    expect(await within(sheet).findByRole("table")).toBeTruthy();
  });

  it("edits a row under its own model and lane", async () => {
    const user = userEvent.setup();
    const { posts } = mount();
    const sheet = await open(user, "gemini");
    await user.click(
      await within(sheet).findByRole("button", {
        name: "Edit gemini-3.5-flash",
      }),
    );
    expect(within(sheet).queryByLabelText("Model")).toBeNull();
    expect(within(sheet).getByLabelText("Input $/M")).toHaveProperty(
      "value",
      "0.3",
    );
    await user.clear(within(sheet).getByLabelText("Input $/M"));
    await user.type(within(sheet).getByLabelText("Input $/M"), "0.4");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));
    await vi.waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]?.body).toMatchObject({
      model_id: "gemini-3.5-flash",
      lane: "chat",
      input_per_mtok: "0.4",
    });
  });

  it("names a model in use that has no price, and prices it in its own lane", async () => {
    const user = userEvent.setup();
    const { posts } = mount();
    const sheet = await open(user, "gemini");
    expect(
      await within(sheet).findByText(
        /gemini-embedding-001 is in use and has no price/,
      ),
    ).toBeTruthy();
    await user.click(within(sheet).getByRole("button", { name: "Set price" }));
    expect(within(sheet).getByLabelText("Model")).toHaveProperty(
      "value",
      "gemini-embedding-001",
    );
    await user.type(within(sheet).getByLabelText("Input $/M"), "0.15");
    await user.type(within(sheet).getByLabelText("Output $/M"), "0");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));
    await vi.waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]?.body).toMatchObject({
      model_id: "gemini-embedding-001",
      lane: "embeddings",
    });
  });

  it("says a refresh left this vendor to be priced by hand", async () => {
    const user = userEvent.setup();
    mount();
    await user.click(
      await screen.findByRole("button", { name: "Refresh model prices" }),
    );
    const sheet = await open(user, "gemini");
    expect(await within(sheet).findByText("Set by hand")).toBeTruthy();
  });

  it("offers a reader who may not write the sheet no verb at all", async () => {
    const user = userEvent.setup();
    mount({ ai_routing: ["read"], ai_model_rate: ["read"] });
    const sheet = await open(user, "gemini");
    await within(sheet).findByText("gemini-3.5-flash");
    expect(
      within(sheet).queryByRole("button", { name: /Add price|Edit/ }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Refresh model prices" }),
    ).toBeNull();
  });
});
