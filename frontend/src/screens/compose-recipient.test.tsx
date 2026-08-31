/** @vitest-environment jsdom */
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
import { ComposeModal } from "./compose";

// Who the composer says a reply goes to, before a draft is asked for.
//
// Sending REQUIRES a recipient, so an empty To field is not a missing
// nicety: it is a reply the reader must address by hand against a thread
// that already names the person. These tests assert what stands in the
// field on open, and that a reader's own typing survives it.

type Sent = { key: string; body: unknown };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

const THREAD_RECIPIENT = {
  full_name: "Dietmar Rietsch",
  first_name: "Dietmar",
  address: "dietmar@buyer.test",
};

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
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
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, body });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      return jsonResponse({});
    }),
  );
  return sent;
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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ComposeModal recipient", () => {
  it("addresses the reply to the thread's counterparty on open", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    // No click first: the address stands there before any drafting, which is
    // the whole difference from filling it out of the draft response.
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();
  });

  it("leaves the field empty for a contact with no address on record", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse({
          full_name: "Dietmar Rietsch",
          first_name: "Dietmar",
          address: "",
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The composer settles without inventing one. Rendering the empty string
    // as a chip would put an unsendable recipient in front of the reader.
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Draft with AI" }),
      ).toBeTruthy();
    });
    expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
  });

  it("keeps the recipient the reader typed", async () => {
    // Answers slowly enough that the reader types first, which is the race
    // this guards: a prefill landing after them must not replace their choice.
    let release: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubRoutes({
      "GET /activities/act-1/reply-recipient": async () => {
        await held;
        return jsonResponse(THREAD_RECIPIENT);
      },
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    const to = await screen.findByLabelText("To");
    await user.type(to, "someone.else@buyer.test{Enter}");
    release?.();

    expect(await screen.findByText("someone.else@buyer.test")).toBeTruthy();
    await waitFor(() => {
      expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
    });
  });
});
