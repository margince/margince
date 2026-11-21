// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../../i18n";
import { PreferenceCenterScreen } from "../settings/preferences";
import { ConfirmDetailsScreen } from "./confirm";
import { UnsubscribeScreen } from "./unsubscribe";

function stubEdge(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

function show(ui: React.ReactNode, locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const CENTER = {
  masked_email: "m@example.com",
  workspace_name: "Gradion",
  refused: [],
  purposes: [
    {
      key: "marketing_email",
      label: "Marketing email",
      state: "granted",
      locked: false,
      grant_needs_confirmation: false,
      choice: "opt_in",
      can_opt_in: true,
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  document.title = "Margince";
});

describe("the unsubscribe page names the purpose in the reader's words", () => {
  it.each([
    ["en", "Product news"],
    ["de", "Produktneuigkeiten"],
    ["vi", "Tin tức sản phẩm"],
  ] as const)("in %s", async (locale, name) => {
    stubEdge(200, CENTER);
    show(<UnsubscribeScreen token="t" purpose="marketing_email" />, locale);
    expect(await screen.findByText(name)).toBeInTheDocument();
    expect(screen.queryByText("Marketing email")).not.toBeInTheDocument();
    expect(screen.queryByText(/marketing_email/)).not.toBeInTheDocument();
  });

  it("never echoes link text for a purpose the server does not know", async () => {
    stubEdge(200, CENTER);
    const { container } = show(
      <UnsubscribeScreen token="t" purpose="%3Cb%3Ex%3C%2Fb%3E" />,
    );
    expect(
      await screen.findByText("This link matches no email type"),
    ).toBeInTheDocument();
    expect(container.textContent).not.toContain("%3C");
  });
});

describe("the dead-link pages share one layout", () => {
  const cases: [string, () => React.ReactNode, number][] = [
    ["unsubscribe", () => <UnsubscribeScreen />, 0],
    ["preferences", () => <PreferenceCenterScreen />, 0],
    ["confirm", () => <ConfirmDetailsScreen />, 0],
    ["preferences, token refused", () => <PreferenceCenterScreen token="x" />, 404],
    ["confirm, token refused", () => <ConfirmDetailsScreen token="x" />, 404],
  ];
  it.each(cases)("%s shows the heading and the tab title", async (_n, ui, status) => {
    if (status) stubEdge(status, { title: "not found" });
    show(ui());
    expect(
      await screen.findByRole("heading", {
        name: "This link is no longer valid",
      }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(document.title).toBe("This link is no longer valid"),
    );
  });
});
