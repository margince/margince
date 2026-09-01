/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { UnsubscribeScreen } from "./unsubscribe";

// The page the VISIBLE unsubscribe link in a message opens. Like the
// preference centre next door, this file must NOT seed a workspace slug:
// the token in the URL is the whole capability, and seeding one would mask
// a regression where the client starts sending workspace context anyway.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const CENTER = {
  masked_email: "m•••••@example.com",
  workspace_name: "Gradion",
  refused: [],
  purposes: [
    {
      key: "transactional",
      label: "Deal & service messages",
      state: "granted",
      locked: true,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: false,
    },
    {
      key: "business_correspondence",
      label: "Direct correspondence",
      state: "unknown",
      locked: false,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: true,
    },
  ],
};

type Sent = { key: string; url: string };

function stubEdge(
  overrides: Record<string, () => Response | Promise<Response>> = {},
  sent: Sent[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push({ key, url: url.pathname + url.search });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /public/preferences/tok-123")
        return jsonResponse(CENTER);
      if (key === "POST /public/preferences/tok-123/unsubscribe") {
        return jsonResponse({ unsubscribed: ["business_correspondence"] });
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UnsubscribeScreen", () => {
  // The rule the whole page exists for. Mail scanners and link prefetchers
  // follow links in a mailbox with nobody present, so arriving must never
  // withdraw anything.
  it("writes nothing on arrival — only reads", async () => {
    const sent = stubEdge();
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );

    expect(
      await screen.findByRole("button", {
        name: /unsubscribe from these emails/i,
      }),
    ).toBeInTheDocument();
    expect(sent.every((r) => r.key.startsWith("GET "))).toBe(true);
  });

  it("names the kind of email the link is about", async () => {
    stubEdge();
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );
    expect(
      await screen.findByText("Direct correspondence"),
    ).toBeInTheDocument();
  });

  it("withdraws only the purpose the link named, on an explicit press", async () => {
    const sent = stubEdge();
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: /unsubscribe from these emails/i,
      }),
    );

    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: /unsubscribed/i }),
      ).toBeInTheDocument(),
    );
    const post = sent.find((r) => r.key.startsWith("POST "));
    expect(post?.url).toContain("purpose=business_correspondence");
  });

  // The server reports what it CHANGED, so an empty array is a replay
  // rather than a failure — and the page must say so instead of showing a
  // fresh confirmation for a no-op.
  it("reads an empty result as already unsubscribed, not as an error", async () => {
    stubEdge({
      "POST /public/preferences/tok-123/unsubscribe": () =>
        jsonResponse({ unsubscribed: [] }),
    });
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: /unsubscribe from these emails/i,
      }),
    );
    expect(
      await screen.findByRole("heading", { name: /already switched off/i }),
    ).toBeInTheDocument();
  });

  // A control that always fails is worse than an absent one.
  it("offers no button for a purpose that cannot be switched off", async () => {
    stubEdge();
    render(<UnsubscribeScreen token="tok-123" purpose="transactional" />);
    expect(
      await screen.findByText(/can't be switched off/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /unsubscribe from these emails/i }),
    ).not.toBeInTheDocument();
  });

  it("treats an unknown token as one neutral sentence", async () => {
    stubEdge({
      "GET /public/preferences/tok-123": () =>
        jsonResponse({ title: "not found" }, 404),
    });
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );
    expect(await screen.findByText(/no longer valid/i)).toBeInTheDocument();
    // Nothing to retry on a dead link: a retry button would invite the
    // reader to hammer a link that will never work.
    expect(
      screen.queryByRole("button", { name: /try again/i }),
    ).not.toBeInTheDocument();
  });

  it("explains a rate limit and offers a retry", async () => {
    stubEdge({
      "GET /public/preferences/tok-123": () =>
        jsonResponse({ title: "slow down" }, 429),
    });
    render(
      <UnsubscribeScreen token="tok-123" purpose="business_correspondence" />,
    );
    expect(await screen.findByText(/too many/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
  });

  // A purpose the catalog does not carry must not read as a broken page.
  it("sends a reader whose purpose is unknown to their full preferences", async () => {
    stubEdge();
    render(<UnsubscribeScreen token="tok-123" purpose="no_such_purpose" />);
    expect(
      await screen.findByText(/doesn't name a kind of email/i),
    ).toBeInTheDocument();
  });

  it("early-returns honestly when the address carries no purpose", () => {
    stubEdge();
    render(<UnsubscribeScreen token="tok-123" />);
    expect(screen.getByText(/no longer valid/i)).toBeInTheDocument();
  });
});
