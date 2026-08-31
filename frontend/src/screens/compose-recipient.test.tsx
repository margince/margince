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

  it("sends only to the address the reader typed, when they typed it first", async () => {
    // RecipientField holds typed text in its OWN state until Enter, so `to`
    // is still empty mid-word. A prefill landing in that window would be
    // added, the reader's own address added on commit, and the reply sent to
    // somebody they never chose.
    //
    // Two things prevent that and this asserts the OUTCOME rather than either
    // mechanism: the first keystroke stops the offer (onEditing), and the
    // fill itself never replaces a non-empty field. Neither alone is provable
    // from here — with one removed the other still holds the outcome — so
    // what is pinned is the field's final contents, which is what sends.
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
    // Typed but deliberately NOT committed — no Enter, no blur.
    await user.type(to, "someone.else@buyer.test");
    release?.();

    // Commit what was typed, the way a reader does, and assert the field
    // holds ONLY their address. Asserting the absence mid-word would pass on
    // a render that simply had not happened yet.
    await user.keyboard("{Enter}");
    expect(await screen.findByText("someone.else@buyer.test")).toBeTruthy();
    await waitFor(() => {
      expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
    });
  });

  it("lets the reader remove the prefilled recipient and have it stay gone", async () => {
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

    const user = userEvent.setup();
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: /dietmar@buyer.test/ }),
    );

    // Offering it again on the next render would make the field impossible to
    // clear: the reader deletes it, the effect sees an empty field, and puts
    // the same address straight back.
    await waitFor(() => {
      expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
    });
  });
});
