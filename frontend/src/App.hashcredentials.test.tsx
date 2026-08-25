/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type { Screen } from "./app/router";
import { forgetHashCredential } from "./app/router";
import { LocaleProvider } from "./i18n";
import { memoryStorage } from "./testing/appharness";

// The two emailed links that carry a credential in the hash — the Deal Room's
// buyer invitation and the password reset — through the REAL App, which is
// where this belongs: the defect was never in either screen. App renders a gate
// ahead of every route, so while the release-skew screen holds, neither screen
// mounts and a scrub that lived in one of them never ran. The credential then
// stayed in the address bar and the history entry for as long as the
// installation was mid-upgrade.
//
// The cases run TABLE-DRIVEN over both links, because the second link is how
// the first one's fix was missed: a carrier added to app/router.tsx's table and
// not to this one is a carrier nobody proved anything about.
//
// Each test asserts on `location.hash` at a moment fixed with respect to the
// gate, not merely "eventually".

// The bundle's own release, which is empty in every local build and therefore
// never skews. Supplied so the gate can be put into the one state this suite is
// about; the probe, the comparison and the screen are all the real ones.
vi.mock("./app/release", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./app/release")>()),
  SPA_RELEASE: "1970.41",
}));

type Sent = { key: string; body: unknown };

const PASSWORD = "correct-horse-battery";

const ROOM = {
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

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Both flows' endpoints answer; everything else 503s, so nothing renders from a
// call these surfaces should not need.
function stubApi(): Sent[] {
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
            ? await request.json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, body });
      if (key === "POST /public/rooms/exchange") {
        return jsonResponse({
          session_token: "mdrs_session",
          expires_at: "2026-08-29T00:00:00Z",
        });
      }
      if (key === "GET /public/rooms/me") {
        return jsonResponse(ROOM);
      }
      if (key === "GET /public/rooms/documents") {
        return jsonResponse({ data: [] });
      }
      if (key === "GET /public/rooms/threads") {
        return jsonResponse({ data: [] });
      }
      if (key === "POST /auth/reset-password") {
        return new Response(null, { status: 204 });
      }
      return jsonResponse({ code: "unavailable" }, 503);
    }),
  );
  return sent;
}

// One carrier: the address a credential arrives in, the screen that proves it
// arrived, and what that screen then puts on the wire. `spend` is what the
// reader does to send it — nothing, for a room link the screen exchanges by
// itself.
type Carrier = {
  screen: Screen;
  what: string;
  address: (credential: string) => string;
  scrubbed: string;
  onScreen: () => Promise<unknown>;
  spend: (user: ReturnType<typeof userEvent.setup>) => Promise<void>;
  wire: string;
  sends: (credential: string) => unknown;
};

const CARRIERS: readonly Carrier[] = [
  {
    screen: "room",
    what: "the Deal Room invitation",
    address: (credential) => `#/room?c=${credential}`,
    scrubbed: "#/room",
    onScreen: () => screen.findByRole("heading", { name: "Acme rollout" }),
    spend: async () => undefined,
    wire: "POST /public/rooms/exchange",
    sends: (credential) => ({ credential }),
  },
  {
    screen: "reset-password",
    what: "the password reset link",
    address: (credential) => `#/reset-password?token=${credential}`,
    scrubbed: "#/reset-password",
    onScreen: () =>
      screen.findByRole("heading", { name: "Choose a new password" }),
    spend: async (user) => {
      await user.clear(screen.getByLabelText("New password"));
      await user.type(screen.getByLabelText("New password"), PASSWORD);
      await user.click(
        screen.getByRole("button", { name: "Set new password" }),
      );
    },
    wire: "POST /auth/reset-password",
    sends: (credential) => ({ token: credential, new_password: PASSWORD }),
  },
];

// The api's release, seeded rather than fetched, so the gate has its answer on
// the FIRST render: a probe that resolved a task later would let the route —
// and the screen with it — render first, and the test would then be asserting
// about a screen that had scrubbed the credential itself.
function mount(apiRelease?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  if (apiRelease !== undefined) {
    client.setQueryData(["auth-capabilities"], {
      password: true,
      password_reset: false,
      oidc_providers: [],
      release_version: apiRelease,
    });
  }
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <App />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The credential the test put in the address, so it can be taken back out of
// module memory afterwards. The router holds it there deliberately — that is
// the fix — and one left behind would be handed to the next test's screen.
let planted: { screen: Screen; credential: string } | null = null;

function arriveWith(carrier: Carrier, credential: string) {
  planted = { screen: carrier.screen, credential };
  globalThis.location.hash = carrier.address(credential);
}

function sentTo(sent: Sent[], carrier: Carrier): unknown[] {
  return sent.filter((one) => one.key === carrier.wire).map((one) => one.body);
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (planted !== null) {
    forgetHashCredential(planted.screen, planted.credential);
    planted = null;
  }
  globalThis.sessionStorage.clear();
  globalThis.location.hash = "";
});

describe.each(CARRIERS)("$what, and the gates above the route", (carrier) => {
  it("is out of the address in the same render that puts the skew screen up", () => {
    arriveWith(carrier, "cred_midupgrade");
    const sent = stubApi();

    mount("1970.42");

    // Both assertions are SYNCHRONOUS, before anything async has had a turn:
    // one render pass has happened, it rendered the gate instead of the route,
    // and the address is already clean. Awaiting either would let a later scrub
    // pass for this one.
    expect(
      screen.getByText("This installation is part-way through an update"),
    ).toBeTruthy();
    expect(globalThis.location.hash).toBe(carrier.scrubbed);
    expect(globalThis.location.href).not.toContain("cred_midupgrade");
    // And it was not spent to achieve that: the screen that spends it never
    // mounted, which is the whole premise of the defect.
    expect(sentTo(sent, carrier)).toEqual([]);
  });

  // The gate is up, so nothing is mounted that could notice a link, and a
  // second one arrives anyway. Only the router's hash subscription is left to
  // take it — which is why it takes the credential before it tells React the
  // address moved.
  it("takes a second link that arrives while the gate holds", async () => {
    arriveWith(carrier, "cred_midupgrade");
    const sent = stubApi();

    mount("1970.42");
    arriveWith(carrier, "cred_pasted");
    globalThis.dispatchEvent(new HashChangeEvent("hashchange"));

    await waitFor(() =>
      expect(globalThis.location.hash).toBe(carrier.scrubbed),
    );
    expect(globalThis.location.href).not.toContain("cred_pasted");
    expect(
      screen.getByText("This installation is part-way through an update"),
    ).toBeTruthy();
    expect(sentTo(sent, carrier)).toEqual([]);
  });

  it("still reaches the screen through memory on a cold load", async () => {
    arriveWith(carrier, "cred_first");
    const sent = stubApi();
    const user = userEvent.setup();

    mount();

    // Scrubbed on the first render here too — the screen's chunk has not even
    // been fetched yet.
    expect(globalThis.location.hash).toBe(carrier.scrubbed);
    await carrier.onScreen();
    await carrier.spend(user);
    await waitFor(() =>
      expect(sentTo(sent, carrier)).toEqual([carrier.sends("cred_first")]),
    );
  });

  // A second link pasted into an open tab is a hash change and no remount for
  // the screen already showing. The router takes the new credential out of the
  // address before React is told the address moved, and the screen — still
  // mounted — is handed it and uses it in place of the first.
  it("delivers a second link that arrives by hashchange", async () => {
    arriveWith(carrier, "cred_first");
    const sent = stubApi();
    const user = userEvent.setup();

    mount();
    await carrier.onScreen();

    arriveWith(carrier, "cred_second");
    globalThis.dispatchEvent(new HashChangeEvent("hashchange"));
    await carrier.onScreen();
    await carrier.spend(user);

    await waitFor(() =>
      expect(sentTo(sent, carrier).at(-1)).toEqual(
        carrier.sends("cred_second"),
      ),
    );
    expect(globalThis.location.hash).toBe(carrier.scrubbed);
    expect(globalThis.location.href).not.toContain("cred_second");
  });
});

// The control for all of the above: the scrub is for an address that carries a
// credential, and there is exactly one rewrite of the address bar for a
// carrier. An ordinary route must come through the same code untouched.
it("leaves an ordinary address alone", async () => {
  const replaceState = vi.spyOn(globalThis.history, "replaceState");
  globalThis.location.hash = "#/deals/01J9ZK";
  stubApi();

  mount();

  expect(globalThis.location.hash).toBe("#/deals/01J9ZK");
  // The session probe 503s, so the app settles on the availability screen —
  // awaited so this covers the whole first-load sequence rather than only the
  // render that beat it.
  await screen.findByRole("alert");
  expect(globalThis.location.hash).toBe("#/deals/01J9ZK");
  expect(replaceState).not.toHaveBeenCalled();
});
