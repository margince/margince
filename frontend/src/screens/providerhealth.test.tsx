/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiProviderKeysCard } from "./ai-provider-keys";
import { ProviderHealthCard } from "./providerhealth";
import { jsonResponse, render } from "./settings.testkit";

// One endpoint feeds the System health card and the Providers list, so the two
// surfaces are exercised against the same stub.

const NOW = new Date("2026-10-05T12:00:00Z");

const DIAGNOSER: GrantSpec = {
  ai_diagnostics: ["read"],
  ai_routing: ["read"],
};

type Entry = {
  provider: string;
  health: string;
  since: string;
  retry_after?: string;
};

function stubBackend(providers: Entry[], allow: GrantSpec = DIAGNOSER) {
  const sent: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(req.url).pathname.replace(/^\/v1/, "");
      sent.push(path);
      if (path === "/me") return jsonResponse(meFixture({ allow }));
      if (path === "/ai/provider-health") return jsonResponse({ providers });
      if (path === "/ai/provider-keys")
        return jsonResponse({
          providers: [
            {
              provider: "gemini",
              configured: true,
              env_var: "GEMINI_API_KEY",
              usable: true,
              optional: false,
            },
            {
              provider: "openai",
              configured: true,
              env_var: "OPENAI_API_KEY",
              usable: true,
              optional: false,
            },
          ],
        });
      return jsonResponse({}, 404);
    }),
  );
  return sent;
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("ProviderHealthCard", () => {
  it("says every provider is answering when the list is empty", async () => {
    stubBackend([]);
    render(<ProviderHealthCard />);
    expect(
      await screen.findByText("All AI providers are answering."),
    ).toBeInTheDocument();
  });

  it.each([
    ["degraded", "Degraded", /Some requests .* failing/, /status page/],
    ["down", "Unreachable", /not reachable/, /status page/],
    ["out_of_credit", "Out of credit", /no credit left/, /Top up the provider/],
    [
      "unauthorized",
      "Key rejected",
      /rejected the API key/,
      /Replace the key, or contact your system administrator/,
    ],
  ])(
    "labels %s and tells the reader who can fix it",
    async (health, label, reason, fix) => {
      stubBackend([
        { provider: "openai", health, since: "2026-10-05T09:00:00Z" },
      ]);
      render(<ProviderHealthCard />);
      const notice = await screen.findByTestId("ai-provider-health-openai");
      expect(within(notice).getByText(label)).toBeInTheDocument();
      expect(within(notice).getByText(reason)).toBeInTheDocument();
      expect(within(notice).getByText(fix)).toBeInTheDocument();
      expect(notice).toHaveTextContent("Started 3 hours ago");
      expect(notice).not.toHaveTextContent("Next check");
    },
  );

  it("dates the next check when the provider is blocked until then", async () => {
    stubBackend([
      {
        provider: "openai",
        health: "out_of_credit",
        since: "2026-10-05T11:30:00Z",
        retry_after: "2026-10-05T12:15:00Z",
      },
    ]);
    render(<ProviderHealthCard />);
    const notice = await screen.findByTestId("ai-provider-health-openai");
    expect(notice).toHaveTextContent("Started 30 minutes ago");
    expect(notice).toHaveTextContent("Next check in 15 minutes");
  });

  it("says the check is due once the retry time has passed", async () => {
    stubBackend([
      {
        provider: "openai",
        health: "down",
        since: "2026-10-05T11:00:00Z",
        retry_after: "2026-10-05T11:55:00Z",
      },
    ]);
    render(<ProviderHealthCard />);
    expect(await screen.findByText(/Next check is due/)).toBeInTheDocument();
  });

  it("is withheld, and asks nothing, without the diagnostics grant", async () => {
    const sent = stubBackend([], { ai_routing: ["read"] });
    render(<ProviderHealthCard />);
    expect(
      await screen.findByText(/requires a permission your role does not have/),
    ).toBeInTheDocument();
    expect(sent).not.toContain("/ai/provider-health");
  });
});

describe("Providers list", () => {
  it("shows the health badge beside the configuration badge on the affected row", async () => {
    stubBackend([
      {
        provider: "openai",
        health: "unauthorized",
        since: "2026-10-05T10:00:00Z",
      },
    ]);
    render(<AiProviderKeysCard />);
    const row = await screen.findByTestId("ai-provider-row-openai");
    expect(await screen.findByText("Key rejected")).toBeInTheDocument();
    // The configuration badge stays on the line; the health badge is a second
    // one in the same row, never a replacement for it.
    expect(within(row).getByText("Ready")).toBeInTheDocument();
    const panelRow = row.parentElement;
    if (!panelRow) throw new Error("the provider line has no row");
    expect(
      within(panelRow).getByTestId("ai-provider-health-openai"),
    ).toHaveTextContent("Key rejected");
    expect(
      within(screen.getByTestId("ai-provider-row-gemini")).queryByText(
        "Key rejected",
      ),
    ).toBeNull();
    expect(screen.getAllByTestId(/^ai-provider-health-/)).toHaveLength(1);
  });

  it("draws no health without the diagnostics grant, and does not ask", async () => {
    const sent = stubBackend([], { ai_routing: ["read"] });
    render(<AiProviderKeysCard />);
    await screen.findByTestId("ai-provider-row-openai");
    expect(sent).not.toContain("/ai/provider-health");
  });
});
