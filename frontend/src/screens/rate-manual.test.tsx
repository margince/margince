/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { RefreshModelPrices } from "./rate-catalogue-refresh";
import { ModelPriceDialog } from "./rate-manual";

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
const OTHER_ROW = {
  ...GEMINI_ROW,
  provider: "anthropic",
  model_id: "claude-x",
};

type Posted = { url: string; body: unknown };

// A backend for the dialog: the sheet, the vendor's list, and the write. The
// write either answers with the row it stored or with the given refusal.
function backend(
  posts: Posted[],
  refuse?: { status: number; body: unknown },
  allow: GrantSpec = { ai_model_rate: ["read", "create", "update"] },
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request?.url ?? input);
    const method = request?.method ?? init?.method ?? "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow, seat: "full" }));
    }
    if (url.includes("/v1/ai/available-models/")) {
      return jsonResponse({
        provider: "gemini",
        models: [{ id: "gemini-4-pro" }],
      });
    }
    if (method === "POST" && url.endsWith("/v1/ai-model-rates")) {
      const body = request ? await request.json() : {};
      posts.push({ url, body });
      if (refuse) {
        return jsonResponse(refuse.body, refuse.status);
      }
      return jsonResponse({ ...GEMINI_ROW, ...body }, 201);
    }
    if (url.includes("/v1/ai-model-rates")) {
      return jsonResponse({ data: [GEMINI_ROW, OTHER_ROW] });
    }
    if (url.endsWith("/v1/ai-model-rates/refresh")) {
      return jsonResponse({ providers: [] });
    }
    return jsonResponse({}, 404);
  });
}

function mount(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { invalidate };
}

describe("ModelPriceDialog for one provider", () => {
  it("tabulates only that provider's priced models and loads one to edit", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    mount(<ModelPriceDialog provider="gemini" onClose={() => {}} />);

    const table = await screen.findByRole("table");
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual([
      "Model",
      "Input $/M",
      "Output $/M",
      "Cache read $/M",
      "Cache write $/M",
      "Edit",
    ]);
    const row = within(table)
      .getAllByText("gemini-3.5-flash")[0]
      ?.closest("tr");
    if (!row) throw new Error("the model has no row");
    expect(within(row).getByText("2.5")).toBeTruthy();
    expect(within(table).queryByText("claude-x")).toBeNull();

    await user.click(
      within(row).getByRole("button", { name: "Edit gemini-3.5-flash" }),
    );
    expect(screen.getByLabelText("Input $/M")).toHaveProperty("value", "0.3");
    expect(screen.getByLabelText("Output $/M")).toHaveProperty("value", "2.5");
    // The row being edited is marked, so the form is not read as a new price.
    expect(row.className).toContain("row-current");
  });

  it("files a new model under the lane chosen and edits a row under its own", async () => {
    const user = userEvent.setup();
    const posts: Posted[] = [];
    vi.stubGlobal("fetch", backend(posts));
    mount(<ModelPriceDialog provider="gemini" onClose={() => {}} />);

    await user.type(screen.getByLabelText("Model"), "embed-9");
    await user.click(screen.getByRole("combobox", { name: "Used for" }));
    await user.click(screen.getByRole("option", { name: "Embeddings" }));
    await user.type(screen.getByLabelText("Input $/M"), "0.1");
    await user.type(screen.getByLabelText("Output $/M"), "0");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByRole("status");
    expect(posts[0]?.body).toMatchObject({
      model_id: "embed-9",
      lane: "embeddings",
    });

    await user.click(
      await screen.findByRole("button", { name: "Edit gemini-3.5-flash" }),
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await vi.waitFor(() => expect(posts).toHaveLength(2));
    expect(posts[1]?.body).toMatchObject({
      model_id: "gemini-3.5-flash",
      lane: "chat",
    });
  });

  it("writes the price as typed, zero included, and shows what it saved", async () => {
    const user = userEvent.setup();
    const posts: Posted[] = [];
    vi.stubGlobal("fetch", backend(posts));
    const { invalidate } = mount(
      <ModelPriceDialog provider="ollama" onClose={() => {}} />,
    );

    await user.type(screen.getByLabelText("Model"), "gemma4");
    await user.type(screen.getByLabelText("Input $/M"), "0");
    await user.type(screen.getByLabelText("Output $/M"), "0");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("status")).toBeTruthy();
    expect(posts).toHaveLength(1);
    expect(posts[0]?.body).toMatchObject({
      provider: "ollama",
      model_id: "gemma4",
      input_per_mtok: "0",
      output_per_mtok: "0",
      cache_read_per_mtok: "0",
      cache_write_per_mtok: "0",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["ai-model-rates"] });
  });

  it("refuses a malformed price before sending it", async () => {
    const user = userEvent.setup();
    const posts: Posted[] = [];
    vi.stubGlobal("fetch", backend(posts));
    mount(<ModelPriceDialog provider="gemini" onClose={() => {}} />);

    await user.type(screen.getByLabelText("Model"), "m");
    await user.type(screen.getByLabelText("Input $/M"), "1,5");
    await user.type(screen.getByLabelText("Output $/M"), "2");

    expect(screen.getByRole("alert").textContent).toMatch(/plain numbers/);
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(posts).toEqual([]);
  });

  it("speaks the server's refusal and keeps the form", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backend([], {
        status: 422,
        body: {
          type: "https://errors.gradion.com/validation",
          title: "Unprocessable",
          status: 422,
          code: "rate_past",
          detail: "effective_date cannot be in the past",
        },
      }),
    );
    mount(<ModelPriceDialog provider="gemini" onClose={() => {}} />);
    await user.type(screen.getByLabelText("Model"), "m");
    await user.type(screen.getByLabelText("Input $/M"), "1");
    await user.type(screen.getByLabelText("Output $/M"), "2");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Rate not saved")).toBeTruthy();
    expect(screen.getByLabelText("Model")).toHaveProperty("value", "m");
  });
});

describe("the refresh report's Set by hand rows", () => {
  const REPORT = {
    providers: [
      {
        provider: "gemini",
        outcome: "not_available",
        updated: 0,
        unchanged: 0,
        models: [],
        unlisted: [],
      },
      {
        provider: "openai_compatible",
        outcome: "updated",
        updated: 1,
        unchanged: 0,
        models: ["a/b"],
        unlisted: [],
      },
    ],
  };

  async function refreshed(allow: GrantSpec) {
    const user = userEvent.setup();
    const fetchMock = backend([], undefined, allow);
    vi.stubGlobal(
      "fetch",
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : null;
        if (
          (request?.url ?? String(input)).endsWith("/ai-model-rates/refresh")
        ) {
          return jsonResponse(REPORT);
        }
        return fetchMock(input, init);
      },
    );
    mount(<RefreshModelPrices />);
    await user.click(
      screen.getByRole("button", { name: "Refresh model prices" }),
    );
    await screen.findByRole("table");
    return user;
  }

  it("offers Edit prices on a vendor with no price list, and opens its dialog", async () => {
    const user = await refreshed({
      ai_model_rate: ["read", "create", "update"],
    });
    const button = await screen.findByRole("button", {
      name: "Edit prices gemini",
    });
    expect(
      screen.queryByRole("button", { name: "Edit prices openai_compatible" }),
    ).toBeNull();
    await user.click(button);
    expect(
      await screen.findByRole("heading", { name: "Set prices for gemini" }),
    ).toBeTruthy();
  });

  it("offers nothing to a reader who may not write the sheet", async () => {
    await refreshed({ ai_model_rate: ["read"] });
    expect(screen.queryByRole("button", { name: /Edit prices/ })).toBeNull();
  });
});
