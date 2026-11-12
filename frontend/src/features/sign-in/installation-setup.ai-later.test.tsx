// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { AiSetupPanel } from "./installation-setup.ai-later";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const BINDER: GrantSpec = { ai_routing: ["read", "update"] };

function mount(allow: GrantSpec, modelConfigured: boolean) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/installation/setup")) {
        return jsonResponse({
          complete: modelConfigured,
          steps: [
            { step: "ai_models", configured: modelConfigured, blocking: true },
            { step: "oauth_app", configured: true, blocking: false },
          ],
        });
      }
      return jsonResponse({ data: [] });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({ allow }));
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const view = render(<AiSetupPanel />, { wrapper: Wrap });
  return { ...view, client };
}

describe("AiSetupPanel", () => {
  it("offers the first-run form to a reader who can bind, while no model is bound", async () => {
    mount(BINDER, false);
    expect(
      await screen.findByRole("heading", { name: "Set up AI" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Connect provider" }),
    ).toBeInTheDocument();
    // Settings has nothing to skip to.
    expect(screen.queryByRole("button", { name: "Not now" })).toBeNull();
  });

  it("steps aside once a model is bound", async () => {
    const { container, client } = mount(BINDER, true);
    await waitFor(() =>
      expect(client.getQueryState(["installation-setup"])?.status).toBe(
        "success",
      ),
    );
    expect(container.innerHTML).toBe("");
  });

  it("draws nothing for a reader who cannot bind models", async () => {
    const { container } = mount({ ai_routing: ["read"] }, false);
    expect(container.innerHTML).toBe("");
  });
});
