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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { OAuthConsent } from "./oauthconsent";

// The consent screen is where a human hands an agent their own authority.
// The nonce that proves the redirect was real is NOT in the endpoint's
// response — the consent cookie that pairs with it is Path=/oauth/authorize
// and never reaches any endpoint the SPA calls — so every test arrives via
// the same route the server actually uses: a realistic redirect fragment.

const NONCE = "n1";

// Every fixture carries the RFC 8707 audience param the server armed the
// request with: a redirect fragment that omits it cannot prove the screen
// carries it forward, and `resource` is the one authorize param whose loss
// would be silent — the flow still completes, bound to the wrong audience.
const RESOURCE = "https://margince.example/mcp";

// The whole authorize request minus the nonce — what a re-entry (the
// post-sign-in retry) must carry, spelled once so both assertions read from
// the same list.
const AUTHORIZE_KEYS = [
  "response_type",
  "client_id",
  "redirect_uri",
  "scope",
  "code_challenge",
  "code_challenge_method",
  "resource",
  "state",
];

function hashWith(overrides: Record<string, string> = {}): string {
  const params = new URLSearchParams({
    response_type: "code",
    client_id: "client-1",
    redirect_uri: "https://client.example/cb",
    scope: "read",
    code_challenge: "abc123",
    code_challenge_method: "S256",
    resource: RESOURCE,
    state: "night-state",
    consent: NONCE,
    ...overrides,
  });
  return `#/oauth-consent?${params.toString()}`;
}

// A TERMINAL refusal: the request comes back with a marker and no nonce,
// because nothing the human could submit would be accepted any more. The same
// shape as the not-signed-in redirect (which carries no marker either), and the
// shape a nonce is never minted client-side to fill in.
function hashWithError(
  errorCode: string,
  overrides: Record<string, string> = {},
): string {
  const params = new URLSearchParams({
    response_type: "code",
    client_id: "client-1",
    redirect_uri: "https://client.example/cb",
    scope: "read",
    code_challenge: "abc123",
    code_challenge_method: "S256",
    resource: RESOURCE,
    state: "night-state",
    error: errorCode,
    ...overrides,
  });
  return `#/oauth-consent?${params.toString()}`;
}

function hashWithoutNonce(overrides: Record<string, string> = {}): string {
  const params = new URLSearchParams({
    response_type: "code",
    client_id: "client-1",
    redirect_uri: "https://client.example/cb",
    scope: "read",
    code_challenge: "abc123",
    code_challenge_method: "S256",
    resource: RESOURCE,
    state: "night-state",
    ...overrides,
  });
  return `#/oauth-consent?${params.toString()}`;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type ConsentPayload = {
  client_name: string;
  offline: boolean;
  scopes: string[];
};

function stubConsent(payload: ConsentPayload) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname === "/v1/oauth/consent-request") {
        return jsonResponse(payload);
      }
      return jsonResponse({ title: "not found" }, 404);
    }),
  );
}

// Every read this screen makes fails. The states that need no server data must
// still render: a refusal whose whole content IS the refusal cannot be gated
// behind a fetch that the same cause often breaks.
function stubConsentUnavailable() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ title: "not found" }, 404)),
  );
}

// Answers GET /v1/me — the ONE signal the re-entry effect is allowed to act
// on. hasSession=false 401s exactly like an anonymous visitor: the same shape
// useMe() maps to "no session" for the app's own auth gate, so a test that
// starts from here proves the effect reads the real signal rather than
// assuming one.
function stubSession(hasSession: boolean) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname === "/v1/me") {
        return hasSession
          ? jsonResponse({ user: { id: "u1" }, roles: [], teams: [] })
          : jsonResponse({ title: "unauthorized" }, 401);
      }
      return jsonResponse({ title: "not found" }, 404);
    }),
  );
}

function renderWithClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

function render(ui: ReactNode) {
  renderWithClient(ui);
}

function renderConsent(
  payload: Partial<ConsentPayload> & { scopes: string[] },
) {
  stubConsent({
    client_name: payload.client_name ?? "Claude Code",
    offline: payload.offline ?? false,
    scopes: payload.scopes,
  });
  render(<OAuthConsent />);
}

// The ONE way a test observes what the approve form would post: a real
// <form method="post" action="/oauth/authorize">, never fetch, so this reads
// the form's own hidden fields exactly as the browser would send them.
function approveForm(): HTMLFormElement {
  const button = screen.getByRole("button", { name: /authorize/i });
  const form = button.closest("form");
  if (!(form instanceof HTMLFormElement)) {
    throw new Error("the Authorize button is not inside a form");
  }
  return form;
}

// Captures a real <form method="post" action="/oauth/authorize"> submit —
// the flow is a native browser POST, not fetch, so a test observes it by
// listening for the submit event and reading the form's own data, exactly as
// the browser would send it. preventDefault avoids jsdom's unimplemented
// navigation from firing on a submit this suite doesn't let complete.
function stubAuthorizePost(): { body: URLSearchParams } {
  const posted = { body: new URLSearchParams() };
  document.addEventListener(
    "submit",
    (event) => {
      event.preventDefault();
      const form = event.target;
      if (!(form instanceof HTMLFormElement)) {
        throw new Error("submit fired on something that is not a form");
      }
      posted.body = new URLSearchParams(
        [...new FormData(form).entries()].map(([key, value]) => [
          key,
          String(value),
        ]),
      );
    },
    { once: true },
  );
  return posted;
}

beforeEach(() => {
  globalThis.location.hash = hashWith();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("OAuthConsent", () => {
  it("offers every scope ticked, and grants what stays ticked", async () => {
    renderConsent({ scopes: ["read", "draft", "write", "send", "enrich"] });
    const user = userEvent.setup();

    // Everything is ticked by default: a connection that can only read is not
    // what someone connecting an assistant is asking for, and the first thing
    // they try would fail in a way that reads as the product being broken.
    for (const label of [
      "Read records",
      "Draft messages",
      "Change records",
      "Send messages",
      "Buy contact data",
    ]) {
      expect(
        await screen.findByRole("checkbox", { name: new RegExp(label) }),
      ).toBeChecked();
    }

    await user.click(screen.getByRole("checkbox", { name: /Send messages/ }));
    await user.click(
      screen.getByRole("checkbox", { name: /Buy contact data/ }),
    );

    expect(approveForm().querySelector('input[name="scopes"]')).toHaveValue(
      "read draft write",
    );
  });

  it("refuses to approve nothing", async () => {
    renderConsent({ scopes: ["read", "draft", "write", "send", "enrich"] });
    const user = userEvent.setup();
    await screen.findByRole("checkbox", { name: /Read records/ });
    for (const label of [
      /Read records/,
      /Draft messages/,
      /Change records/,
      /Send messages/,
      /Buy contact data/,
    ]) {
      await user.click(screen.getByRole("checkbox", { name: label }));
    }

    // The server refuses an empty set too (parseConsentedScopes) — but it does
    // so at the POST, and the human is still here, so the screen says it
    // first.
    expect(screen.getByRole("button", { name: /authorize/i })).toBeDisabled();
    expect(screen.getByText(/Pick at least one, or deny/)).toBeInTheDocument();
  });

  it("still denies without a scope selection", async () => {
    renderConsent({ scopes: ["read"] });
    // Deny is never gated on the ticks: a human refusing a client must not
    // have to satisfy a form first.
    expect(
      await screen.findByRole("button", { name: /deny access/i }),
    ).toBeEnabled();
  });

  it("names the client from the server, never from the URL", async () => {
    globalThis.location.hash = hashWith({ client_name: "EVIL" });
    renderConsent({ client_name: "Claude Code", scopes: ["read"] });
    expect(await screen.findByText(/Claude Code/)).toBeTruthy();
    expect(screen.queryByText(/EVIL/)).toBeNull();
  });

  it("posts the granted scopes and the nonce the redirect handed it", async () => {
    const posted = stubAuthorizePost();
    renderConsent({ scopes: ["read", "write"] });
    await userEvent.click(
      await screen.findByRole("button", { name: /authorize/i }),
    );
    expect(posted.body.get("scopes")).toBe("read write");
    // The nonce comes from the fragment, not from the endpoint: the cookie
    // that holds its counterpart is Path=/oauth/authorize and reaches
    // nothing else.
    expect(posted.body.get("consent")).toBe(NONCE);
  });

  it("posts the deny marker when the human refuses the connection", async () => {
    const posted = stubAuthorizePost();
    renderConsent({ scopes: ["read"] });
    await userEvent.click(
      await screen.findByRole("button", { name: /deny access/i }),
    );
    expect(posted.body.get("deny")).toBe("1");
    expect(posted.body.get("consent")).toBe(NONCE);
  });

  it("discloses a self-renewing connection", async () => {
    renderConsent({ offline: true, scopes: ["read"] });
    expect(
      await screen.findByText(/stay connected without asking again/i),
    ).toBeTruthy();
  });
});

describe("OAuthConsent — what a refused consent is handed back", () => {
  it("renders stale_consent as a dead end with no approve control", async () => {
    globalThis.location.hash = hashWithError("stale_consent");
    stubConsentUnavailable();
    render(<OAuthConsent />);
    expect(await screen.findByText(/request has expired/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /authorize/i })).toBeNull();
    // The nonce is spent forever, so the recovery is the client, never a
    // reload of this page — the copy says so rather than staying silent.
    expect(
      await screen.findByText(/reloading this page will not help/i),
    ).toBeTruthy();
  });

  it("gives the stale_consent dead end a way back into the app", async () => {
    globalThis.location.hash = hashWithError("stale_consent");
    stubConsentUnavailable();
    render(<OAuthConsent />);
    await userEvent.click(
      await screen.findByRole("button", { name: /back to margince/i }),
    );
    // A rail-less screen with no forward action still needs an exit — this
    // is the app's own "home" route, never the client's callback.
    expect(globalThis.location.hash).toBe("#/home");
  });

  it("renders invalid_request even though the consent-request read fails", async () => {
    globalThis.location.hash = hashWithError("invalid_request");
    // The likeliest cause of invalid_request is a client that went unknown,
    // disabled or deleted — which is exactly what makes this read 404. A card
    // rendered behind that read becomes "couldn't load this view" with a Retry
    // button, replacing the one sentence that tells the human what to do.
    stubConsentUnavailable();
    render(<OAuthConsent />);
    expect(await screen.findByText(/could not be completed/i)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /authorize/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /retry/i })).toBeNull();
    expect(
      screen.getByRole("button", { name: /back to margince/i }),
    ).toBeTruthy();
  });
});

describe("OAuthConsent — re-entering after sign-in", () => {
  it("re-enters /oauth/authorize once signed in, when the fragment carries no nonce", async () => {
    globalThis.location.hash = hashWithoutNonce();
    stubSession(true);
    const assigned: string[] = [];
    vi.stubGlobal("location", {
      ...globalThis.location,
      assign: (url: string) => assigned.push(url),
    });
    render(<OAuthConsent />);
    await waitFor(() => expect(assigned).toHaveLength(1));
    const reentered = new URLSearchParams(assigned[0].split("?")[1] ?? "");
    expect(assigned[0]).toContain("/oauth/authorize?");
    // A fresh nonce is only ever minted server-side, so replaying an absent
    // one is not on offer.
    expect(reentered.has("consent")).toBe(false);
    expect([...reentered.keys()].sort()).toEqual([...AUTHORIZE_KEYS].sort());
    expect(reentered.get("resource")).toBe(RESOURCE);
  });

  it("never re-enters without a session — the effect's own signal never fires", async () => {
    globalThis.location.hash = hashWithoutNonce();
    stubSession(false);
    const assigned: string[] = [];
    vi.stubGlobal("location", {
      ...globalThis.location,
      assign: (url: string) => assigned.push(url),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <OAuthConsent />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    // Wait for the /me probe to fully settle to its OTHER terminal state
    // (error) before asserting the negative — proving the effect had every
    // chance to fire and still didn't, not just that it hasn't caught up
    // yet. Loop-freedom rests on this: `me.data` is the only signal the
    // effect reacts to, and an anonymous visitor never produces it.
    await waitFor(() =>
      expect(client.getQueryState(["me"])?.status).toBe("error"),
    );
    expect(assigned).toEqual([]);
  });
});

// The 2026-07-28 profile makes this a MUST on this screen, and a loopback one
// an extra warning. Both exist for the CIMD case: a client id is a URL and a
// client name is whatever that URL's document says, so the destination is the
// one fact about a connection only the human can judge — and a document
// cannot prove which program is listening on a port on this machine.
describe("the redirect disclosure", () => {
  it("names the host the authorization is sent back to", async () => {
    globalThis.location.hash = hashWith({
      redirect_uri: "https://client.example/cb",
    });
    renderConsent({ scopes: ["read"] });

    expect(await screen.findByText(/client\.example/)).toBeInTheDocument();
    expect(
      screen.queryByText(/address on this computer/),
    ).not.toBeInTheDocument();
  });

  it("warns when that destination is an address on this computer", async () => {
    globalThis.location.hash = hashWith({
      redirect_uri: "http://127.0.0.1:3000/callback",
    });
    renderConsent({ scopes: ["read"] });

    expect(await screen.findByText(/127\.0\.0\.1:3000/)).toBeInTheDocument();
    expect(screen.getByText(/address on this computer/)).toBeInTheDocument();
  });
});
