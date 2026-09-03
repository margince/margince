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

  // A FIRST message to a record. Every path into the To field ran through an
  // activity, so a lead nobody has written to yet opened its composer empty
  // over a record whose address was on screen behind the drawer.
  it("addresses a first message to the record's own address", async () => {
    stubRoutes();
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@example.test"
        open
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText("dung.ly@example.test")).toBeTruthy();
  });

  // The thread outranks the record. An address recorded on the message being
  // answered is who THAT conversation is with; the record's primary address is
  // not, and a reply sent to it would leave the thread it answers.
  it("prefers the thread's counterparty over the record's own address", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@example.test"
        open
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();
    expect(screen.queryByText("dung.ly@example.test")).toBeNull();
  });

  // A record with no address on file offers nothing, rather than an empty chip
  // the reader has to notice and delete.
  it("offers nothing when the record has no address", async () => {
    stubRoutes();
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress={undefined}
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByLabelText("To");
    // The committed recipients are the chips in the To row. Read as elements
    // rather than as text: an assertion on the absence of one particular
    // address would pass over a chip carrying any other.
    expect(
      document.querySelectorAll(".recipient-field .chips li"),
    ).toHaveLength(0);
  });

  // The offer follows the conversation. A reader who picks a different one is
  // owed ITS counterparty, and the previous one leaves on the change itself —
  // not when the new address happens to resolve, because a lookup still out
  // or a contact with no address would otherwise leave the old counterparty
  // standing as the recipient of a reply to somebody else.
  it("replaces the offered recipient when the conversation changes", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /activities/act-2/reply-recipient": () =>
        jsonResponse({
          full_name: "Petra Novak",
          first_name: "Petra",
          address: "petra@buyer.test",
        }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const composer = (activityId: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ComposeModal
            activityId={activityId}
            entityType="person"
            entityId="p-1"
            open
            onClose={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const view = rtlRender(composer("act-1"));
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();

    view.rerender(composer("act-2"));
    expect(await screen.findByText("petra@buyer.test")).toBeTruthy();
    expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
  });

  it("drops the offered recipient when the new conversation has no address", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /activities/act-2/reply-recipient": () =>
        jsonResponse({
          full_name: "Petra Novak",
          first_name: "Petra",
          address: "",
        }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const composer = (activityId: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ComposeModal
            activityId={activityId}
            entityType="person"
            entityId="p-1"
            open
            onClose={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const view = rtlRender(composer("act-1"));
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();

    view.rerender(composer("act-2"));
    // Gone on the change, and nothing invented in its place: the reply to the
    // second conversation is not addressed to the first one's correspondent.
    await waitFor(() => {
      expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
    });
    expect(screen.queryByText("petra@buyer.test")).toBeNull();
  });

  // Two conversations with one counterparty. The change vacates the offer,
  // but the address the lookup resolves does not move, and the cache makes
  // that sharp: a cached answer never passes back through "unknown". The offer
  // is owed all the same, or the field is empty for a reply to somebody the
  // thread names.
  it("offers the address again when the next conversation has the same counterparty", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /activities/act-2/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["compose-reply-recipient", "act-2"], THREAD_RECIPIENT);
    const composer = (activityId: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ComposeModal
            activityId={activityId}
            entityType="person"
            entityId="p-1"
            open
            onClose={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const view = rtlRender(composer("act-1"));
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();

    view.rerender(composer("act-2"));
    await waitFor(() => {
      expect(screen.getByText("dietmar@buyer.test")).toBeTruthy();
    });
  });

  // The slot is ours; what the reader put beside it is theirs. A change of
  // conversation swaps the one address the composer offered and touches
  // nothing the reader committed.
  it("keeps the recipient the reader added when the conversation changes", async () => {
    stubRoutes({
      "GET /activities/act-1/reply-recipient": () =>
        jsonResponse(THREAD_RECIPIENT),
      "GET /activities/act-2/reply-recipient": () =>
        jsonResponse({
          full_name: "Petra Novak",
          first_name: "Petra",
          address: "petra@buyer.test",
        }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const composer = (activityId: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ComposeModal
            activityId={activityId}
            entityType="person"
            entityId="p-1"
            open
            onClose={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const view = rtlRender(composer("act-1"));
    expect(await screen.findByText("dietmar@buyer.test")).toBeTruthy();

    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("To"),
      "colleague@buyer.test{Enter}",
    );
    expect(await screen.findByText("colleague@buyer.test")).toBeTruthy();

    view.rerender(composer("act-2"));
    expect(await screen.findByText("petra@buyer.test")).toBeTruthy();
    expect(screen.getByText("colleague@buyer.test")).toBeTruthy();
    expect(screen.queryByText("dietmar@buyer.test")).toBeNull();
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
