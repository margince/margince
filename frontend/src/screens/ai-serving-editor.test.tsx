// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { summarize } from "./ai-request-summary";
import { jsonResponse, render } from "./ai-routing.testkit";
import type { SliceValue } from "./ai-routing-slice";
import { ServingSection, servingBlocked } from "./ai-serving-editor";

// The serving editor in the binding dialog: when it applies, what the server's
// preview says about the text, and the request it shows for the tier.

type Routing = components["schemas"]["AiRouting"];

const ROUTING: Routing = {
  profile: "cloud_frontier",
  tiers: {
    cheap_cloud: {
      provider: "openai_compatible",
      model: "openai/gpt-oss-120b",
      base_url: "https://openrouter.ai/api",
    },
  },
  embeddings: {
    provider: "openai_compatible",
    model: "mistralai/mistral-embed-2312",
  },
  providers: {
    openai_compatible: {
      base_url: "https://openrouter.ai/api",
      upstream: { zdr: true },
    },
  },
};

const TIER: SliceValue = {
  kind: "tier",
  tier: "cheap_cloud",
  binding: ROUTING.tiers.cheap_cloud,
};

const SCHEMA = {
  openRouterProvider: {
    properties: {
      sort: {
        description: "Order candidate hosts.",
        "x-doc-url": "https://openrouter.ai/docs",
        "x-placement": "tier",
        oneOf: [],
      },
      zdr: {
        description: "Zero data retention.",
        type: "boolean",
        "x-placement": "connection",
      },
    },
  },
  openRouterReasoning: {
    properties: {
      effort: { enum: ["low", "high"], description: "The budget." },
    },
  },
};

function previewServer(answer: (body: Routing) => unknown) {
  const sent: Routing[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me"))
        return jsonResponse(
          meFixture({ allow: { ai_routing: ["read", "update"] } }),
        );
      if (req.url.includes("/ai/routing/schema")) return jsonResponse(SCHEMA);
      if (req.url.includes("/ai/routing/preview")) {
        const body: Routing = await req.json();
        sent.push(body);
        return jsonResponse(answer(body));
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    }),
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function type(text: string) {
  fireEvent.change(screen.getByRole("textbox", { name: "Serving JSON" }), {
    target: { value: text },
  });
}

describe("servingBlocked", () => {
  it("names why a lane takes no serving block", () => {
    expect(servingBlocked(TIER, ROUTING)).toBeNull();
    expect(
      servingBlocked({ kind: "decisions", binding: undefined }, ROUTING),
    ).toBe("decisions");
    expect(
      servingBlocked(
        { ...TIER, binding: { provider: "gemini", model: "m" } },
        ROUTING,
      ),
    ).toBe("provider");
    const direct = {
      ...ROUTING,
      providers: { openai_compatible: { base_url: "https://api.mistral.ai" } },
    };
    expect(
      servingBlocked(
        { ...TIER, binding: { provider: "openai_compatible", model: "m" } },
        direct,
      ),
    ).toBe("host");
  });
});

describe("ServingSection", () => {
  it("says why a direct vendor gets no editor", () => {
    previewServer(() => ({}));
    render(
      <ServingSection
        value={{ ...TIER, binding: { provider: "gemini", model: "m" } }}
        routing={ROUTING}
        disabled={false}
        onChange={() => {}}
        onValid={() => {}}
      />,
    );
    expect(screen.queryByRole("textbox", { name: "Serving JSON" })).toBeNull();
    expect(screen.getByText(/gemini serves this model itself/)).toBeTruthy();
  });

  it("lists each problem the server names, on its line, and holds the save", async () => {
    previewServer(() => ({
      current_version: "v1",
      features: [],
      unused_tiers: [],
      errors: [
        {
          field: "tiers.cheap_cloud.routing.provider.sort.by",
          code: "setting_invalid",
          message: "must be one of price, throughput, latency.",
        },
        {
          field: "tiers.premium.routing.provider",
          code: "setting_invalid",
          message: "another lane's problem",
        },
      ],
    }));
    const onValid = vi.fn();
    render(
      <ServingSection
        value={TIER}
        routing={ROUTING}
        disabled={false}
        onChange={() => {}}
        onValid={onValid}
      />,
    );

    type(`{\n  "provider": {\n    "sort": { "by": "fastest" }\n  }\n}`);

    expect(await screen.findByText("1 problem")).toBeTruthy();
    expect(screen.getByText("provider.sort.by")).toBeTruthy();
    expect(screen.getByText("L3")).toBeTruthy();
    expect(screen.queryByText("another lane's problem")).toBeNull();
    expect(onValid).toHaveBeenLastCalledWith(false);
  });

  it("refuses text that does not parse without asking the server", async () => {
    const sent = previewServer(() => ({}));
    render(
      <ServingSection
        value={TIER}
        routing={ROUTING}
        disabled={false}
        onChange={() => {}}
        onValid={() => {}}
      />,
    );

    type(`{ "provider": `);

    expect(await screen.findByText("1 problem")).toBeTruthy();
    expect(
      sent.every((body) => body.tiers.cheap_cloud.routing === undefined),
    ).toBe(true);
  });

  it("shows the request the server merged, each key with where it came from", async () => {
    const sent = previewServer(() => ({
      current_version: "v1",
      features: [],
      unused_tiers: [],
      effective: {
        tiers: { cheap_cloud: { provider: { sort: "latency", zdr: true } } },
      },
    }));
    const onChange = vi.fn();
    const onValid = vi.fn();
    render(
      <ServingSection
        value={TIER}
        routing={ROUTING}
        disabled={false}
        onChange={onChange}
        onValid={onValid}
      />,
    );

    type(`{"provider":{"sort":"latency"}}`);

    expect(await screen.findByText("Pick the host by latency.")).toBeTruthy();
    expect(
      screen.getByText("Only hosts that keep no copy of prompts or replies."),
    ).toBeTruthy();
    expect(screen.getByText("You set here")).toBeTruthy();
    expect(screen.getByText("Connection")).toBeTruthy();
    expect(
      screen.getByText(
        "Thinking level comes from each task’s setting under AI tasks.",
      ),
    ).toBeTruthy();
    expect(onChange).toHaveBeenLastCalledWith({
      provider: { sort: "latency" },
    });
    await waitFor(() => expect(onValid).toHaveBeenLastCalledWith(true));
    expect(sent.at(-1)?.tiers.cheap_cloud.routing).toEqual({
      provider: { sort: "latency" },
    });
    expect(await screen.findByText("connection only")).toBeTruthy();
  });

  it("empties to the shipped default", async () => {
    previewServer(() => ({
      current_version: "v1",
      features: [],
      unused_tiers: [],
    }));
    const onChange = vi.fn();
    render(
      <ServingSection
        value={{
          ...TIER,
          binding: {
            ...TIER.binding,
            routing: { provider: { sort: "price" } },
          },
        }}
        routing={ROUTING}
        disabled={false}
        onChange={onChange}
        onValid={() => {}}
      />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Use shipped default" }),
    );
    expect(onChange).toHaveBeenLastCalledWith(undefined);
    expect(await screen.findByText("Shipped default")).toBeTruthy();
  });
});

describe("summarize", () => {
  it("credits the connection with its keys whatever the tier wrote", () => {
    const lines = summarize(
      {
        provider: { zdr: true, sort: { by: "price", partition: "none" } },
        reasoning: { effort: "low" },
      },
      {
        provider: { sort: { by: "price", partition: "none" } },
        reasoning: { effort: "low" },
      },
      true,
    );
    expect(lines.map((l) => [l.key, l.source])).toEqual([
      ["provider.zdr", "connection"],
      ["provider.sort", "tier"],
      ["reasoning.effort", "tier"],
    ]);
    expect(lines[1].sentence.key).toBe("aiServing.say.sortAcross");
  });

  it("calls a key the tier did not write the shipped default, and says the task decides an unset effort", () => {
    const lines = summarize(
      { provider: { quantizations: ["fp16"], max_price: { prompt: 1 } } },
      undefined,
      true,
    );
    expect(lines.map((l) => l.source)).toEqual(["default", "default", "task"]);
    expect(lines[1].sentence.params).toEqual({ prices: "$1 prompt" });
  });
});
