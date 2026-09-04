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
import { BuyerRoomScreen } from "./buyerroom";

// The buyer's Deal Room: the credential in the hash is exchanged once and
// scrubbed, the session rides as a Bearer on every call, and each access
// state gets its own screen. What went out on the wire is the contract.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Sent = { key: string; authorization: string | null; body: unknown };

function stubRoom(
  overrides: Record<string, () => Response> = {},
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
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({
        key,
        authorization: request?.headers.get("Authorization") ?? null,
        body,
      });
      const override = overrides[key];
      if (override) return override();
      if (key === "POST /public/rooms/exchange") {
        return jsonResponse({
          session_token: "mdrs_session",
          expires_at: "2026-08-29T00:00:00Z",
        });
      }
      if (key === "GET /public/rooms/me") {
        return jsonResponse(LIVE);
      }
      if (key === "GET /public/rooms/documents") {
        return jsonResponse({
          data: [
            {
              id: "d-1",
              group_key: "legal",
              title: "Data processing agreement",
              position: 0,
              filename: "DPA_v7.pdf",
            },
          ],
        });
      }
      if (key === "GET /public/rooms/threads") {
        return jsonResponse({
          data: [
            {
              id: "th-1",
              room_id: "r-1",
              document_id: "d-1",
              required_change: false,
              state: "open",
              author: { side: "seller", name: "Ada Admin" },
              created_at: "2026-08-22T09:00:00Z",
              comments: [
                {
                  id: "c-1",
                  thread_id: "th-1",
                  body: "Does clause 4 work for you?",
                  author: { side: "seller", name: "Ada Admin" },
                  created_at: "2026-08-22T09:00:00Z",
                },
              ],
            },
          ],
        });
      }
      if (key === "POST /public/rooms/threads/th-1/comments") {
        return jsonResponse({ id: "th-1" });
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

const LIVE = {
  access: "live",
  participant: {
    id: "p-1",
    full_name: "Laura Buyer",
    email: "laura@buyer.example",
    capability: "comment",
  },
  steward_name: "Ada Admin",
  room: {
    title: "Acme rollout",
    welcome_message: "Welcome, Laura.",
    release_no: 1,
    released_at: "2026-08-22T09:00:00Z",
    steward_name: "Ada Admin",
  },
};

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
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  globalThis.sessionStorage.clear();
  globalThis.location.hash = "";
});

describe("BuyerRoomScreen", () => {
  it("exchanges the credential from the link, scrubs it, and reads the room as a Bearer", async () => {
    globalThis.location.hash = "#/room?c=mdr_secret";
    const sent = stubRoom();
    render(<BuyerRoomScreen />);

    await screen.findByRole("heading", { name: "Acme rollout" });
    expect(screen.getByText("Welcome, Laura.")).toBeInTheDocument();
    await screen.findByText("Data processing agreement");
    expect(screen.getByText("Legal")).toBeInTheDocument();

    expect(globalThis.location.hash).toBe("#/room");
    const exchange = sent.find((s) => s.key === "POST /public/rooms/exchange");
    expect(exchange?.body).toEqual({ credential: "mdr_secret" });
    const me = sent.find((s) => s.key === "GET /public/rooms/me");
    expect(me?.authorization).toBe("Bearer mdrs_session");
    expect(globalThis.sessionStorage.getItem("margince.room.session")).toBe(
      "mdrs_session",
    );
  });

  // The credential no longer waits in the address bar for the screen to read
  // it — the router takes it as it reads the hash and holds it in memory, which
  // is what keeps it out of history while a gate above the route renders
  // instead. Memory outlives a mount, so the screen has to empty it as it
  // spends it: a link is single-use, and a remount that sent it again would be
  // refused and would put the dead-link page over the session it had just
  // opened.
  it("spends a link once, however often the screen mounts", async () => {
    globalThis.location.hash = "#/room?c=mdr_once";
    const sent = stubRoom();
    const first = render(<BuyerRoomScreen />);
    await screen.findByRole("heading", { name: "Acme rollout" });
    first.unmount();

    render(<BuyerRoomScreen />);

    await screen.findByRole("heading", { name: "Acme rollout" });
    expect(
      sent.filter((s) => s.key === "POST /public/rooms/exchange"),
    ).toHaveLength(1);
  });

  it("the buyer reads the seller's question and answers it with the Bearer", async () => {
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    const sent = stubRoom();
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await screen.findByText("Does clause 4 work for you?");
    await user.type(screen.getByLabelText("Reply"), "Thirty days would.");
    await user.click(screen.getByRole("button", { name: "Reply" }));
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "POST /public/rooms/threads/th-1/comments"),
      ).toBeTruthy(),
    );
    const posted = sent.find(
      (s) => s.key === "POST /public/rooms/threads/th-1/comments",
    );
    expect(posted?.body).toEqual({ body: "Thirty days would." });
    expect(posted?.authorization).toBe("Bearer mdrs_session");
  });

  it("a paused room shows the paused notice and no content", async () => {
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ ...LIVE, access: "paused", room: undefined }),
    });
    render(<BuyerRoomScreen />);

    await screen.findByText("Access is paused");
    expect(
      screen.getByText(/Ada Admin has paused this room/),
    ).toBeInTheDocument();
    expect(screen.queryByText("Acme rollout")).not.toBeInTheDocument();
  });

  it("a dead link offers a fresh one by email, and the session is dropped on a 401", async () => {
    globalThis.location.hash = "#/room?c=mdr_used";
    const sent = stubRoom({
      "POST /public/rooms/exchange": () =>
        jsonResponse({ status: 404, code: "not_found" }, 404),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await screen.findByText("This link no longer works");
    await user.type(
      screen.getByLabelText("Your email address"),
      "laura@buyer.example",
    );
    await user.click(
      screen.getByRole("button", { name: /send me a new link/i }),
    );
    await screen.findByText(/a new link is on its way/i);
    const request = sent.find(
      (s) => s.key === "POST /public/rooms/link-request",
    );
    expect(request?.body).toEqual({ email: "laura@buyer.example" });
  });

  it("a session the server refuses is forgotten", async () => {
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_stale");
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ status: 401, code: "unauthorized" }, 401),
    });
    render(<BuyerRoomScreen />);

    await screen.findByText("This link no longer works");
    expect(globalThis.sessionStorage.getItem("margince.room.session")).toBe(
      null,
    );
  });

  // The download used to spell the object-URL dance inline — create, anchor,
  // click, revoke — beside `download.ts`, whose docblock names the revoke as
  // "the step a second copy forgets". Two copies of a four-step sequence is
  // one chance to forget it; this asserts the shared one runs, revoke and all,
  // and that the blob keeps the type the server chose (a PDF handed over as
  // application/octet-stream downloads with the wrong icon and opens in
  // nothing).
  it("hands the buyer a file through the shared download, revoke included", async () => {
    const created: Blob[] = [];
    const createObjectURL = vi.fn((source: Blob | MediaSource) => {
      if (source instanceof Blob) {
        created.push(source);
      }
      return "blob:room";
    });
    const revokeObjectURL = vi.fn();
    // spyOn, not defineProperties: the file's afterEach calls
    // vi.unstubAllGlobals(), which does not undo a defineProperties, so a
    // hand-installed mock would stay on URL for every later test in this file.
    vi.spyOn(URL, "createObjectURL").mockImplementation(createObjectURL);
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(revokeObjectURL);
    stubRoom({
      // BYTES, never a Blob. This file runs in jsdom, so `Blob` is jsdom's
      // while `Response` is Node's own — and undici builds a response body by
      // calling `.stream()` on what it is handed, which jsdom's Blob does not
      // have. The two realms only meet here, in a fixture, so the failure read
      // as the download refusing rather than as the stub never constructing.
      // The type still reaches the reader: `parseAs: "blob"` takes it from the
      // Content-Type header below, which is the same header a server sends.
      "GET /public/rooms/documents/d-1/file": () =>
        new Response(new TextEncoder().encode("%PDF-1.7"), {
          status: 200,
          headers: { "Content-Type": "application/pdf" },
        }),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(
      await screen.findByRole("button", {
        name: /Download Data processing agreement/i,
      }),
    );

    await waitFor(() => expect(createObjectURL).toHaveBeenCalledOnce());
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:room");
    expect(created[0]?.type).toBe("application/pdf");
  });

  // The same file request, refused. The screen's own translated sentence still
  // reaches the reader — a plain `Error` would have been dropped as wording
  // nobody wrote for a user.
  it("says why a refused download did not happen", async () => {
    stubRoom({
      "GET /public/rooms/documents/d-1/file": () =>
        jsonResponse({ code: "not_found" }, 404),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(
      await screen.findByRole("button", {
        name: /Download Data processing agreement/i,
      }),
    );

    expect(
      await screen.findByText(
        "The download did not start. Try again, or ask your contact.",
      ),
    ).toBeTruthy();
  });
});

// Whether the reader may write and why they may not are one decision. They
// were two, and they disagreed: `conversationRefusal` named a preview and the
// write test never mentioned one, so a preview session whose seat carried
// `comment` was handed a working composer while the page told it a preview
// cannot write. Only the server minting every preview seat read-only kept
// that off the screen.
describe("a preview never gets a working composer", () => {
  it("refuses the write even when the seat itself says comment", async () => {
    stubRoom({
      "GET /public/rooms/me": () => jsonResponse({ ...LIVE, preview: true }),
      "GET /public/rooms/documents": () => jsonResponse({ data: [] }),
      "GET /public/rooms/threads": () => jsonResponse({ data: [] }),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    render(<BuyerRoomScreen />);

    // LIVE's participant carries `capability: "comment"`, so a write gate that
    // reads only the capability admits this reader.
    expect(LIVE.participant.capability).toBe("comment");

    const start = await screen.findByRole("button", { name: "New thread" });
    expect(start).toBeDisabled();
    // No open composer anywhere: a disabled button beside a live textarea
    // would still let the reader type and press Post.
    expect(document.querySelectorAll("textarea")).toHaveLength(0);
  });

  it("still gives a commenting buyer a working composer", async () => {
    stubRoom({
      "GET /public/rooms/me": () => jsonResponse(LIVE),
      "GET /public/rooms/documents": () => jsonResponse({ data: [] }),
      "GET /public/rooms/threads": () => jsonResponse({ data: [] }),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    render(<BuyerRoomScreen />);

    // The admit case, without which the refusal above passes against a page
    // that refuses everybody.
    expect(await screen.findByLabelText("New thread")).toBeTruthy();
  });
});

// `BuyerRoomAccess` is a plain string on the wire, not a union, so the
// compiler cannot enumerate the states and a server that grows a fifth one
// reaches this build untouched. A write gate that lists the states it refuses
// hands that state a working composer; one that names the single state it
// admits refuses it. This test supplies a state no build has heard of, which
// is the only way to check the DEFAULT rather than the four known branches.
describe("an access state this build does not know", () => {
  it("cannot write", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ ...LIVE, access: "quarantined" }),
      "GET /public/rooms/documents": () => jsonResponse({ data: [] }),
      "GET /public/rooms/threads": () => jsonResponse({ data: [] }),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    render(<BuyerRoomScreen />);

    const start = await screen.findByRole("button", { name: "New thread" });
    expect(start).toBeDisabled();
    expect(document.querySelectorAll("textarea")).toHaveLength(0);
  });

  it("is not told the room is closed, because that would be a guess", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ ...LIVE, access: "quarantined" }),
      "GET /public/rooms/documents": () => jsonResponse({ data: [] }),
      "GET /public/rooms/threads": () => jsonResponse({ data: [] }),
    });
    globalThis.sessionStorage.setItem("margince.room.session", "mdrs_session");
    render(<BuyerRoomScreen />);

    expect(await screen.findByText("This room is now read-only.")).toBeTruthy();
    expect(
      screen.queryByText(
        "This room is closed; what it shared is a record now.",
      ),
    ).toBeNull();
  });
});
