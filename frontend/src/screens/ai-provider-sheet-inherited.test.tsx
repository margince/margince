/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";

// Gemini on Vertex AI serves Gemini's models, so a model its own sheet does
// not price is priced at Gemini's row. Its sheet lists those rows as borrowed,
// and correcting one writes a Vertex price that overrides it.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const WRITER: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_model_rate: ["read", "create", "update"],
};

const ROW = {
  lane: "chat",
  output_per_mtok: "2.5",
  cache_read_per_mtok: "0.03",
  cache_write_per_mtok: "0",
  effective_date: "2026-08-01",
};

function mount() {
  const posts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(req.url).pathname;
      if (path.endsWith("/me"))
        return jsonResponse(meFixture({ allow: WRITER }));
      if (path.endsWith("/ai/provider-keys")) {
        return jsonResponse({
          providers: [
            {
              provider: "gemini",
              configured: true,
              env_var: "GEMINI_API_KEY",
              usable: true,
              optional: false,
              credential_kind: "api_key",
            },
            {
              provider: "gemini_vertex",
              configured: true,
              env_var: "GEMINI_VERTEX_SA_JSON",
              usable: true,
              optional: false,
              credential_kind: "service_account",
              priced_by: "gemini",
            },
          ],
        });
      }
      if (path.endsWith("/ai/routing")) {
        return jsonResponse({
          profile: "eu_hosted",
          tiers: {
            premium: { provider: "gemini_vertex", model: "gemini-2.5-flash" },
          },
          embeddings: { provider: "gemini", model: "gemini-embedding-001" },
          providers: { gemini_vertex: { location: "eu" } },
        });
      }
      if (path.endsWith("/ai-model-rates/refresh")) {
        return jsonResponse({ providers: [] });
      }
      if (path.endsWith("/ai-model-rates") && req.method === "POST") {
        posts.push(await req.json());
        return jsonResponse({}, 201);
      }
      if (path.endsWith("/ai-model-rates")) {
        return jsonResponse({
          data: [
            {
              ...ROW,
              provider: "gemini",
              model_id: "gemini-2.5-flash",
              input_per_mtok: "0.3",
            },
            {
              ...ROW,
              provider: "gemini",
              model_id: "gemini-3.5-flash",
              input_per_mtok: "1.5",
            },
            {
              ...ROW,
              provider: "gemini_vertex",
              model_id: "gemini-3.5-flash",
              input_per_mtok: "1.6",
            },
          ],
        });
      }
      if (path.includes("/ai/provider-locations/")) {
        return jsonResponse({ provider: "gemini_vertex", locations: [] });
      }
      return jsonResponse({}, 404);
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <LocaleProvider initial="en">
        <AiProviderKeysCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return posts;
}

async function openVertex(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    within(
      await screen.findByTestId("ai-provider-row-gemini_vertex"),
    ).getByRole("button", { name: /^Edit/ }),
  );
  return screen.findByRole("dialog");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a provider priced by another", () => {
  it("lists the borrowed rows beside its own, and says which are borrowed", async () => {
    const user = userEvent.setup();
    mount();
    const sheet = await openVertex(user);

    const borrowed = await within(sheet).findByRole("row", {
      name: /gemini-2\.5-flash/,
    });
    expect(borrowed).toHaveTextContent("From Google Gemini");
    expect(borrowed).toHaveTextContent("0.3");
    const own = within(sheet).getByRole("row", { name: /gemini-3\.5-flash/ });
    expect(own).toHaveTextContent("1.6");
    expect(own).not.toHaveTextContent("From Google Gemini");
    // A model routing binds and a borrowed row prices is not unpriced.
    expect(within(sheet).queryByText(/has no price/)).toBeNull();
  });

  it("writes its own price when a borrowed row is corrected", async () => {
    const user = userEvent.setup();
    const posts = mount();
    const sheet = await openVertex(user);

    await user.click(
      await within(sheet).findByRole("button", {
        name: "Edit gemini-2.5-flash",
      }),
    );
    await user.clear(within(sheet).getByLabelText("Input $/M"));
    await user.type(within(sheet).getByLabelText("Input $/M"), "0.33");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    await vi.waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({
      provider: "gemini_vertex",
      model_id: "gemini-2.5-flash",
      input_per_mtok: "0.33",
    });
  });

  it("links Google's Vertex AI price list", async () => {
    const user = userEvent.setup();
    mount();
    const sheet = await openVertex(user);

    expect(
      await within(sheet).findByRole("link", { name: /Provider price list/ }),
    ).toHaveAttribute(
      "href",
      "https://cloud.google.com/vertex-ai/generative-ai/pricing",
    );
  });
});
