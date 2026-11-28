// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
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

function stubSequence(routes: Record<string, [number, unknown]>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://t.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const [status, body] = routes[
        `${method} ${url.pathname.replace(/^\/v1/, "")}`
      ] ?? [500, {}];
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

const RECORD = {
  kind: "record_confirmation",
  full_name: "Anna Müller",
  title: "Head of Ops",
  company: "Buyer GmbH",
  email: "anna@example.test",
  phone: "+49 30 111",
  marketing_state: "unknown",
  provenance: [],
};

const SUBSCRIPTION = {
  kind: "subscription_confirmation",
  purpose_key: "product_updates",
  purpose_label: "Product updates",
  state: "unknown",
};

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

  it("shows no label for a localized purpose key the server did not return", async () => {
    stubEdge(200, CENTER);
    const { container } = show(
      <UnsubscribeScreen token="t" purpose="transactional" />,
      "de",
    );
    expect(
      await screen.findByText("Dieser Link entspricht keinem E-Mail-Typ"),
    ).toBeInTheDocument();
    expect(container.textContent).not.toContain("Sicherheits");
    expect(container.textContent).not.toContain("transactional");
  });

  it("never echoes link text for a purpose the server does not know", async () => {
    stubEdge(200, CENTER);
    const { container } = show(
      <UnsubscribeScreen token="t" purpose="%3Cb%3Ex%3C%2Fb%3E" />,
    );
    expect(
      await screen.findByText(englishCopy["prefs.unsub.unknownPurposeTitle"]),
    ).toBeInTheDocument();
    for (const sentinel of ["%3C", "<b>x</b>", "b>x</b"]) {
      expect(container.textContent).not.toContain(sentinel);
      expect(document.title).not.toContain(sentinel);
    }
    for (const heading of screen.getAllByRole("heading")) {
      expect(heading.textContent).not.toContain("<b>");
    }
  });
});

describe("a link that dies after the first read lands on the dead-link page", () => {
  it("for a record confirmation answered 404 on the press", async () => {
    stubSequence({
      "GET /public/confirm/tok": [200, RECORD],
      "POST /public/confirm/tok": [404, { title: "not found" }],
    });
    show(<ConfirmDetailsScreen token="tok" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Confirm" }),
    );
    expect(
      await screen.findByRole("heading", {
        name: englishCopy["prefs.unsub.deadLinkTitle"],
      }),
    ).toBeInTheDocument();
  });

  it("for a subscription confirmation answered 404 on the press", async () => {
    stubSequence({
      "GET /public/confirm/tok": [200, SUBSCRIPTION],
      "POST /public/confirm/tok": [404, { title: "not found" }],
    });
    show(<ConfirmDetailsScreen token="tok" />);
    await userEvent.click(await screen.findByRole("button"));
    expect(
      await screen.findByRole("heading", {
        name: englishCopy["prefs.unsub.deadLinkTitle"],
      }),
    ).toBeInTheDocument();
  });
});

describe("the dead-link pages share one layout", () => {
  const cases: [string, () => React.ReactNode, number][] = [
    ["unsubscribe", () => <UnsubscribeScreen />, 0],
    ["confirm", () => <ConfirmDetailsScreen />, 0],
    ["confirm, token refused", () => <ConfirmDetailsScreen token="x" />, 404],
  ];
  it.each(cases)(
    "%s shows the heading and the tab title",
    async (_n, ui, status) => {
      if (status) stubEdge(status, { title: "not found" });
      show(ui());
      expect(
        await screen.findByRole("heading", {
          name: englishCopy["prefs.unsub.deadLinkTitle"],
        }),
      ).toBeInTheDocument();
      await waitFor(() =>
        expect(document.title).toBe(englishCopy["prefs.unsub.deadLinkTitle"]),
      );
    },
  );
});
