/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FilePreviewProvider } from "../../design-system/filepreview";
import { LocaleProvider } from "../../i18n";
import { BuyerRoomScreen } from "./buyerroom";

// The buyer's Deal Room. The credential in the hash is exchanged once and
// scrubbed. The session rides on the server's HttpOnly cookie. Each access
// state gets its own screen. What went out on the wire is the contract.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Sent = {
  key: string;
  authorization: string | null;
  credentials: RequestCredentials | undefined;
  body: unknown;
};

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
        // The generated client sends a Request; the file preview sends a URL
        // and an init. Both are read, so a token sent either way shows.
        authorization:
          request?.headers.get("Authorization") ??
          new Headers(init?.headers).get("Authorization"),
        credentials: request?.credentials ?? init?.credentials,
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
  globalThis.localStorage.clear();
  globalThis.location.hash = "";
});

describe("BuyerRoomScreen", () => {
  it("exchanges the credential from the link, scrubs it, and reads the room on the cookie alone", async () => {
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
    expect(me?.authorization).toBeNull();
    expect(me?.credentials).toBe("include");
    // The token stays in the HttpOnly cookie, where no script on the page reads it.
    const stored = [sessionStorage, localStorage].flatMap((store) =>
      Array.from({ length: store.length }, (_, i) =>
        store.getItem(store.key(i) ?? ""),
      ),
    );
    expect(stored).not.toContain("mdrs_session");
  });

  // A second tab, or the same browser after the tab closed, holds no link and
  // no stored token. The browser still sends the cookie, so the room opens.
  it("a new tab with no link opens the room on the kept session", async () => {
    const sent = stubRoom();
    render(<BuyerRoomScreen />);

    await screen.findByRole("heading", { name: "Acme rollout" });
    expect(sent.some((s) => s.key === "POST /public/rooms/exchange")).toBe(
      false,
    );
  });

  // The kept session may be another room's. A refused link says so rather
  // than opening whatever room the browser last held.
  it("a refused link shows the dead-link page even with a kept session", async () => {
    globalThis.location.hash = "#/room?c=mdr_used";
    const sent = stubRoom({
      "POST /public/rooms/exchange": () =>
        jsonResponse({ status: 404, code: "not_found" }, 404),
    });
    render(<BuyerRoomScreen />);

    await screen.findByText(/The link was already used/);
    expect(screen.queryByRole("heading", { name: "Acme rollout" })).toBeNull();
    expect(sent.some((s) => s.key === "GET /public/rooms/me")).toBe(false);
  });

  // One cookie serves the browser. A link opened in another tab moves it to
  // another room; this tab must neither draw that room nor write into it.
  it("a tab whose room another tab replaced draws nothing and writes nothing", async () => {
    let seat = "p-1";
    const sent = stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({
          ...LIVE,
          participant: { ...LIVE.participant, id: seat },
          room: {
            ...LIVE.room,
            title: seat === "p-1" ? "Acme rollout" : "Other room",
          },
        }),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);
    await screen.findByText("Does clause 4 work for you?");

    seat = "p-2";
    await user.type(screen.getByLabelText("Reply"), "Thirty days would.");
    await user.click(screen.getByRole("button", { name: "Reply" }));

    await screen.findByText("This tab shows an older room");
    expect(screen.queryByRole("heading", { name: "Other room" })).toBeNull();
    expect(
      sent.some((s) => s.key === "POST /public/rooms/threads/th-1/comments"),
    ).toBe(false);
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

  it("the buyer reads the seller's question and answers it on the cookie", async () => {
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
    expect(posted?.authorization).toBeNull();
    expect(posted?.credentials).toBe("include");
  });

  it("a paused room shows the paused notice and no content", async () => {
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

  // Expiry is the room's, not the link's: a new link opens the same ended
  // room. So the page sends the buyer to their contact and offers no link.
  it("an expired room names the contact and offers no new link", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ ...LIVE, access: "expired", room: undefined }),
    });
    render(<BuyerRoomScreen />);

    await screen.findByText("Access has ended");
    expect(
      screen.getByText(
        "Access to this room has expired. Contact Ada Admin if you need it again.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Request new link" }),
    ).toBeNull();
  });

  it("a dead link with no kept session offers a fresh one by email", async () => {
    globalThis.location.hash = "#/room?c=mdr_used";
    const sent = stubRoom({
      "POST /public/rooms/exchange": () =>
        jsonResponse({ status: 404, code: "not_found" }, 404),
      "GET /public/rooms/me": () =>
        jsonResponse({ status: 401, code: "unauthorized" }, 401),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await screen.findByText("This link no longer works");
    const email = screen.getByLabelText(/^Your email address/);
    await user.click(screen.getByRole("button", { name: /request new link/i }));
    expect(screen.getByText("Your email address is required.")).toBeTruthy();
    expect(document.activeElement).toBe(email);
    expect(
      sent.filter((s) => s.key === "POST /public/rooms/link-request"),
    ).toHaveLength(0);
    await user.type(email, "laura@buyer.example");
    await user.click(screen.getByRole("button", { name: /request new link/i }));
    await screen.findByText(/a new link is on its way/i);
    const request = sent.find(
      (s) => s.key === "POST /public/rooms/link-request",
    );
    expect(request?.body).toEqual({ email: "laura@buyer.example" });
  });

  it("a session the server refuses shows the dead-link page", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ status: 401, code: "unauthorized" }, 401),
    });
    render(<BuyerRoomScreen />);

    await screen.findByText("This link no longer works");
  });

  it("signing out ends the session on the server and leaves the room", async () => {
    const sent = stubRoom({
      "POST /public/rooms/sign-out": () => new Response(null, { status: 204 }),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    await screen.findByText("This link no longer works");
    expect(sent.some((s) => s.key === "POST /public/rooms/sign-out")).toBe(
      true,
    );
  });

  // On a shared computer, a sign-out that failed must not look like one that
  // worked: the room stays and the failure shows.
  it("a failed sign-out stays in the room and says it failed", async () => {
    stubRoom({
      "POST /public/rooms/sign-out": () =>
        jsonResponse(
          { status: 500, detail: "The server could not sign you out." },
          500,
        ),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    await screen.findByRole("alert");
    expect(
      screen.getByRole("heading", { name: "Acme rollout" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("This link no longer works")).toBeNull();
  });

  it("hides the open room while a second link's exchange is pending", async () => {
    // The browser's cookie: each exchange the server answers replaces it.
    let cookie = "";
    let answerB: (response: Response) => void = () => undefined;
    const exchangeB = new Promise<Response>((resolve) => {
      answerB = resolve;
    });
    const roomFor = (title: string) =>
      jsonResponse({ ...LIVE, room: { ...LIVE.room, title } });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : null;
        const path = new URL(
          request ? request.url : String(input),
          "https://test.local",
        ).pathname.replace(/^\/v1/, "");
        if (path === "/public/rooms/exchange") {
          const body: unknown = await request?.json();
          const credential = Reflect.get(Object(body), "credential");
          if (credential === "mdr_b") {
            return exchangeB;
          }
          cookie = "mdrs_a";
          return jsonResponse({ session_token: "mdrs_a" });
        }
        if (path === "/public/rooms/me") {
          return roomFor(cookie === "mdrs_b" ? "Room B" : "Room A");
        }
        return jsonResponse({ data: [] });
      }),
    );
    globalThis.location.hash = "#/room?c=mdr_a";
    render(<BuyerRoomScreen />);
    await screen.findByRole("heading", { name: "Room A" });

    act(() => {
      globalThis.history.replaceState(null, "", "/#/room?c=mdr_b");
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });

    expect(screen.queryByRole("heading", { name: "Room A" })).toBeNull();
    expect(screen.getByText("Opening your Deal Room…")).toBeInTheDocument();

    cookie = "mdrs_b";
    answerB(jsonResponse({ session_token: "mdrs_b" }));
    // No read cached under room A's session may answer for room B's.
    await waitFor(() =>
      expect(screen.queryByText("Opening your Deal Room…")).toBeNull(),
    );
    expect(screen.queryByRole("heading", { name: "Room A" })).toBeNull();
    await screen.findByRole("heading", { name: "Room B" });
    expect(screen.queryByRole("heading", { name: "Room A" })).toBeNull();
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
    // happy-dom follows the anchor's blob URL, which leaves the window on
    // about:blank for every later case in this file.
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(
      () => undefined,
    );
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
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(
      await screen.findByRole("button", {
        name: /Download Data processing agreement/i,
      }),
    );

    expect(
      await screen.findByText(
        "The download did not start. Retry, or ask your contact.",
      ),
    ).toBeTruthy();
  });

  it("a download the session no longer admits ends the session", async () => {
    stubRoom({
      "GET /public/rooms/documents/d-1/file": () =>
        jsonResponse({ status: 401, code: "unauthorized" }, 401),
    });
    const user = userEvent.setup();
    render(<BuyerRoomScreen />);

    await user.click(
      await screen.findByRole("button", {
        name: /Download Data processing agreement/i,
      }),
    );

    await screen.findByText("This link no longer works");
  });
});

// Whether the reader may write and why they may not are one decision. They
// were two, and they disagreed: `conversationRefusal` named a preview and the
// write test never mentioned one, so a preview session whose seat carried
// `comment` was handed a working composer while the page told it a preview
// cannot write. Only the server minting every preview seat read-only kept
// that off the screen.
describe("reading a document in place", () => {
  // The preview reads the bytes on the room's cookie, as the list did.
  it("opens a PDF over the room, read on the cookie", async () => {
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:room-read");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
    const sent = stubRoom({
      "GET /public/rooms/documents/d-1/file": () =>
        new Response(new TextEncoder().encode("%PDF-1.7"), {
          status: 200,
          headers: { "Content-Type": "application/pdf" },
        }),
    });
    const user = userEvent.setup();
    render(
      <FilePreviewProvider>
        <BuyerRoomScreen />
      </FilePreviewProvider>,
    );

    await user.click(
      await screen.findByRole("button", {
        name: "Read Data processing agreement",
      }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findByRole("heading", { name: "DPA_v7.pdf" }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        sent.find((call) => call.key === "GET /public/rooms/documents/d-1/file")
          ?.credentials,
      ).toBe("include"),
    );
  });

  // A spreadsheet has no reading here to offer; the download stays its one
  // verb, and no button promises what the browser cannot draw.
  it("keeps the download as the only verb for a file no browser draws", async () => {
    stubRoom({
      "GET /public/rooms/documents": () =>
        jsonResponse({
          data: [
            {
              id: "d-2",
              group_key: "commercial",
              title: "Pricing schedule",
              position: 0,
              filename: "pricing.xlsx",
            },
          ],
        }),
    });
    render(
      <FilePreviewProvider>
        <BuyerRoomScreen />
      </FilePreviewProvider>,
    );

    expect(
      await screen.findByRole("button", { name: "Download Pricing schedule" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Read/ })).toBeNull();
  });
});

describe("the hero the buyer lands on", () => {
  it("marks a live room as live, and says who is on the other end", async () => {
    stubRoom();
    globalThis.location.hash = "#/room?c=cred-hero";
    render(<BuyerRoomScreen />);

    expect(
      await screen.findByRole("heading", { name: "Acme rollout" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Welcome, Laura.")).toBeInTheDocument();
    expect(screen.getByText("Ada Admin")).toBeInTheDocument();
    const pill = screen.getByText("Live").closest(".badge");
    expect(pill).toHaveClass("badge-success");
    expect(pill?.querySelector(".badge-live-dot")).toBeInTheDocument();
  });

  // The contact stands beside the documents as a card, and the page says who
  // is reading it: a room forwarded to a colleague should make plain whose
  // seat the tab is in before they write in it.
  it("names the contact beside the documents and who is signed in", async () => {
    stubRoom();
    render(<BuyerRoomScreen />);

    const side = await screen.findByRole("complementary");
    // Named once, under its eyebrow, not again in a sentence below.
    expect(within(side).getByText("Your contact")).toBeInTheDocument();
    expect(within(side).getByText("Ada Admin")).toBeInTheDocument();
    expect(within(side).queryByText(/Your contact:/)).toBeNull();
    expect(screen.getByText("Signed in as Laura Buyer.")).toBeInTheDocument();
  });

  // A closed room says the day, beside the word: "closed" alone leaves a
  // buyer wondering whether they missed something last week or last year.
  it("dates a closed room", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({
          ...LIVE,
          access: "closed",
          room: { ...LIVE.room, closed_at: "2026-08-22T10:00:00Z" },
        }),
    });
    render(<BuyerRoomScreen />);

    expect(await screen.findByText("Closed")).toBeInTheDocument();
    expect(screen.getByText(/^Closed on .*2026/)).toBeInTheDocument();
  });

  // A pill is a claim about the room, and a build that has never heard of the
  // state it was handed has no claim to make. The room still reads.
  it("makes no claim about a state this build does not know", async () => {
    stubRoom({
      "GET /public/rooms/me": () =>
        jsonResponse({ ...LIVE, access: "quiesced" }),
    });
    globalThis.location.hash = "#/room?c=cred-unknown";
    render(<BuyerRoomScreen />);

    expect(
      await screen.findByRole("heading", { name: "Acme rollout" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Live")).toBeNull();
    expect(screen.queryByText("Closed")).toBeNull();
  });
});

describe("a preview never gets a working composer", () => {
  it("refuses the write even when the seat itself says comment", async () => {
    stubRoom({
      "GET /public/rooms/me": () => jsonResponse({ ...LIVE, preview: true }),
      "GET /public/rooms/documents": () => jsonResponse({ data: [] }),
      "GET /public/rooms/threads": () => jsonResponse({ data: [] }),
    });
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
    render(<BuyerRoomScreen />);

    expect(await screen.findByText("This room is now read-only.")).toBeTruthy();
    expect(
      screen.queryByText(
        "This room is closed. Its content is kept as a record.",
      ),
    ).toBeNull();
  });
});
