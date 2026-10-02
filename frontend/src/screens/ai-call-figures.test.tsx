// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import {
  ProviderCallsLine,
  ProviderRecentCalls,
  TierCallsLine,
  TierRecentCalls,
} from "./ai-call-figures";
import {
  draftOf,
  OpenRouterSettings,
  upstreamOf,
} from "./ai-openrouter-settings";
import { ProviderSettingsForm } from "./ai-provider-settings";
import { jsonResponse, render } from "./ai-routing.testkit";
import { LatencyAgainstTimeout } from "./ai-task-outcome";

// The call figures beside each setting, and the connection's privacy rules.

type Row = components["schemas"]["AiCallStatsRow"];

const row = (key: string, extra: Partial<Row> = {}): Row => ({
  key,
  calls: 98,
  failed: 11,
  timeouts: 4,
  p50_ms: 900,
  p95_ms: 2600,
  tokens_in: 1000,
  tokens_out: 500,
  cost_microusd: 20000,
  unpriced: 0,
  ...extra,
});

const DIAGNOSTICS: GrantSpec = {
  ai_diagnostics: ["read"],
  ai_routing: ["read", "update"],
};

function statsServer(
  rows: (url: URL) => Row[],
  allow: GrantSpec = DIAGNOSTICS,
) {
  const asked: URL[] = [];
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) return jsonResponse(meFixture({ allow }));
      if (req.url.includes("/ai/call-stats")) {
        const url = new URL(req.url);
        asked.push(url);
        return jsonResponse({
          window: url.searchParams.get("window"),
          group: url.searchParams.get("group"),
          rows: rows(url),
        });
      }
      if (req.url.includes("/ai/provider-settings/")) {
        puts.push(await req.json());
        return jsonResponse({});
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    }),
  );
  return { asked, puts };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ProviderCallsLine", () => {
  it("says how many calls failed and how many of those timed out", async () => {
    statsServer(() => [row("openai_compatible")]);
    render(<ProviderCallsLine provider="openai_compatible" />);
    expect(
      await screen.findByText("7 d: 98 calls · 11 failed (4 timeouts)"),
    ).toBeTruthy();
  });

  it("says nothing failed, and names a provider with no calls", async () => {
    statsServer(() => [row("gemini", { failed: 0, timeouts: 0, calls: 1 })]);
    render(
      <>
        <ProviderCallsLine provider="gemini" />
        <ProviderCallsLine provider="anthropic" />
      </>,
    );
    expect(await screen.findByText("7 d: 1 call · 0 failed")).toBeTruthy();
    expect(screen.getByText("No calls in the last 7 days")).toBeTruthy();
  });

  it("draws nothing for a reader who may not see the call record", async () => {
    const { asked } = statsServer(() => [row("gemini")], {
      ai_routing: ["read"],
    });
    const { container } = render(<ProviderCallsLine provider="gemini" />);
    await waitFor(() => expect(container.textContent).toBe(""));
    expect(asked).toEqual([]);
  });
});

describe("TierCallsLine", () => {
  it("joins the tier's week with how it sorts its hosts", async () => {
    statsServer(() => [row("cheap_cloud", { calls: 37 })]);
    render(<TierCallsLine tier="cheap_cloud" sort="throughput" />);
    expect(
      await screen.findByText(
        "7 d: 37 calls · p50 0.9 s · 4 timeouts · sort: throughput",
      ),
    ).toBeTruthy();
  });
});

describe("ProviderRecentCalls", () => {
  it("groups a broker's calls by host and opens the calls behind each row", async () => {
    const { asked } = statsServer((url) =>
      url.searchParams.get("group") === "served_provider"
        ? [row("Cerebras"), row("", { failed: 4, calls: 4 })]
        : [row("openai/gpt-oss-120b")],
    );
    const user = userEvent.setup();
    render(<ProviderRecentCalls provider="openai_compatible" broker />);

    const host = await screen.findByRole("link", { name: "Cerebras" });
    expect(host.getAttribute("href")).toBe(
      "#/settings/model-calls?provider=openai_compatible&served_provider=Cerebras",
    );
    expect(screen.getByText("No host answered").closest("a")).toBeNull();
    expect(screen.getAllByText(/0\.02/)).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: "By model" }));
    expect(
      await screen.findByRole("link", { name: "openai/gpt-oss-120b" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "30 d" }));
    await waitFor(() =>
      expect(asked.at(-1)?.searchParams.get("window")).toBe("30d"),
    );
    expect(asked.at(-1)?.searchParams.get("provider")).toBe(
      "openai_compatible",
    );
  });

  it("offers no host grouping off a broker, and says when a window is empty", async () => {
    statsServer(() => []);
    render(<ProviderRecentCalls provider="gemini" broker={false} />);
    expect(await screen.findByText("No calls in this window.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "By host" })).toBeNull();
    expect(screen.getByRole("button", { name: "By tier" })).toBeTruthy();
  });
});

describe("TierRecentCalls", () => {
  it("lists each host that served the tier this week", async () => {
    const { asked } = statsServer(() => [row("Cerebras")]);
    render(<TierRecentCalls tier="local_small" broker />);
    expect(await screen.findByRole("link", { name: "Cerebras" })).toBeTruthy();
    expect(asked[0].searchParams.get("tier")).toBe("local_small");
    expect(
      screen.getByRole("link", { name: "View calls" }).getAttribute("href"),
    ).toBe("#/settings/model-calls?tier=local_small");
  });

  it("lists models off the broker, where the vendor is the host", async () => {
    const { asked } = statsServer(() => [row("gemini-3.1-flash-lite")]);
    render(<TierRecentCalls tier="premium" broker={false} />);
    const link = await screen.findByRole("link", {
      name: "gemini-3.1-flash-lite",
    });
    expect(link.getAttribute("href")).toBe(
      "#/settings/model-calls?model=gemini-3.1-flash-lite&tier=premium",
    );
    expect(asked[0].searchParams.get("group")).toBe("model");
  });

  it("names a hostless row by whether any call was answered", async () => {
    statsServer(() => [row("", { calls: 41, failed: 0, timeouts: 0 })]);
    render(<TierRecentCalls tier="embed" broker />);
    expect(await screen.findByText("Host not recorded")).toBeTruthy();
    expect(screen.queryByText("No host answered")).toBeNull();
  });
});

describe("OpenRouter settings", () => {
  it("reads the stored keys and writes back only what departs from the broker's defaults", () => {
    const draft = draftOf({
      zdr: true,
      only: ["mistral/eu"],
      allow_fallbacks: false,
    });
    expect(draft).toEqual({
      zdr: true,
      deny: false,
      distill: false,
      fallbacks: false,
      only: "mistral/eu",
      ignore: "",
    });
    expect(
      upstreamOf({ ...draft, deny: true, ignore: " coreweave, coreweave ,, " }),
    ).toEqual({
      zdr: true,
      data_collection: "deny",
      allow_fallbacks: false,
      only: ["mistral/eu"],
      ignore: ["coreweave"],
    });
    expect(upstreamOf(draftOf(undefined))).toBeUndefined();
  });

  it("shows only for an OpenRouter host, and saves the rules with the connection", async () => {
    const { puts } = statsServer(() => []);
    const user = userEvent.setup();
    const onHostChange = vi.fn();
    render(
      <ProviderSettingsForm
        provider="openai_compatible"
        onHostChange={onHostChange}
        canManage
        routing={{
          profile: "cloud_frontier",
          tiers: {},
          embeddings: { provider: "openai_compatible", model: "e" },
          providers: {
            openai_compatible: { base_url: "https://openrouter.ai/api" },
          },
        }}
      />,
    );
    const section = screen.getByRole("region", { name: "OpenRouter settings" });
    expect(
      screen.getByRole("button", { name: "Save connection" }),
    ).toBeDisabled();
    await user.click(
      within(section).getByRole("checkbox", { name: /Zero data retention/ }),
    );
    await user.click(screen.getByRole("button", { name: "Save connection" }));
    await waitFor(() =>
      expect(puts).toEqual([
        { base_url: "https://openrouter.ai/api", upstream: { zdr: true } },
      ]),
    );

    await user.click(screen.getByRole("combobox", { name: "Service" }));
    await user.click(screen.getByRole("option", { name: "Mistral" }));
    expect(
      screen.queryByRole("region", { name: "OpenRouter settings" }),
    ).toBeNull();
    // The sheet's figures follow the host being chosen, not the stored one.
    expect(onHostChange).toHaveBeenLastCalledWith("https://api.mistral.ai");
  });

  it("disables every rule for a reader who may not change it", () => {
    statsServer(() => []);
    render(
      <OpenRouterSettings
        draft={draftOf(undefined)}
        onChange={() => {}}
        disabled
      />,
    );
    expect(
      screen.getByRole("checkbox", { name: /Allow fallbacks/ }),
    ).toBeDisabled();
    expect(
      screen.getByRole("textbox", { name: "Use only these hosts" }),
    ).toBeDisabled();
  });
});

describe("LatencyAgainstTimeout", () => {
  it("draws no limit line for a timeout far above every call", () => {
    statsServer(() => []);
    const { container } = render(
      <LatencyAgainstTimeout
        p50Ms={900}
        p95Ms={2600}
        timeoutMs={300000}
        decision={false}
      />,
    );
    expect(
      screen.getByText("timeout 300 s, far above every call"),
    ).toBeTruthy();
    expect(container.querySelector(".ai-latency-limit")).toBeNull();
  });

  it("says when the slow calls run past the decision timeout", () => {
    statsServer(() => []);
    render(
      <LatencyAgainstTimeout
        p50Ms={900}
        p95Ms={16000}
        timeoutMs={15000}
        decision
      />,
    );
    expect(
      screen.getByText("│ decision model timeout 15 s: p95 is above it"),
    ).toBeTruthy();
  });
});
