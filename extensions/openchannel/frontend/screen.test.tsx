/** @vitest-environment jsdom */

import { LocaleProvider } from "@margince/frontend/app";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import OpenchannelScreen from "./screen";

// The connector's screen, over a stubbed transport.
//
// It is compiled by tsconfig.composed-tests.json — so the fixtures below are
// held against the MERGED contract — and run by `make fe-test-ext`, which
// `make check-fe` calls. What it cannot see is what the server actually sends:
// every body here is a fixture, so a screen and a stub that are wrong in the
// same direction agree. The unit's own Go tests are the other half of that
// pair, and the `curl` assertion below is held against the VERIFIER's rule
// rather than against this screen's own spelling of it.

/** A seat that may open, read and change its own endpoint, and read both lists. */
const FULL_GRANT = {
  seat_type: "full",
  objects: {
    ext_openchannel_endpoint: { read: true, create: true, update: true },
    ext_openchannel_inbound: { read: true },
    ext_openchannel_outbound: { read: true },
  },
};

/** A seat that may look and not touch: no control may render. */
const READ_ONLY_GRANT = {
  seat_type: "full",
  objects: {
    ext_openchannel_endpoint: { read: true },
    ext_openchannel_inbound: { read: true },
    ext_openchannel_outbound: { read: true },
  },
};

/** A seat granted nothing on this unit at all. */
const NO_GRANT = { seat_type: "full", objects: {} };

const ENDPOINT = {
  id: "5f2f7a1e-7a0f-4a3a-9b7a-1c0d3e5f7a9b",
  user_id: "9f1d0c4a-3b2e-4f57-9a10-2c8e6b5d4f31",
  slug: "receive",
  ref: "k7m2q9x4",
  url: "",
  enabled: true,
  inbound_received: 3,
  outbound_sent: 1,
  version: 4,
};

const OPENED = { opened: true, endpoint: ENDPOINT };

const INBOUND_ROW = {
  id: "1a2b3c4d-0000-4000-8000-000000000001",
  nonce: "0a1b2c3d4e5f6071",
  state: "ingested",
  attempts: 1,
  body_bytes: 168,
  sent_at: "2026-05-04T09:00:00Z",
  received_at: "2026-05-04T09:00:01Z",
};

const OUTBOUND_ROW = {
  id: "1a2b3c4d-0000-4000-8000-000000000002",
  delivery_key: "act-91",
  attempt: 1,
  recipient: "Someone Outside",
  outcome: "sent",
  created_at: "2026-05-04T10:00:00Z",
};

type Handler = (body: unknown) => unknown;

function stubTransport(
  authorization: unknown,
  handlers: Readonly<Record<string, Handler>>,
) {
  const calls: { path: string; method: string; body: unknown }[] = [];
  // The client is built with `fetch: (request) => globalThis.fetch(request)`,
  // so the stub is handed ONE Request and no init — reading a body off an init
  // argument records null for every call and makes "what did the screen send"
  // vacuous.
  const fetchStub = async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const json = (value: unknown, status = 200) =>
      new Response(JSON.stringify(value), {
        status,
        headers: { "Content-Type": "application/json" },
      });
    if (url.endsWith("/v1/me")) {
      return json({ user: {}, roles: [], teams: [], authorization });
    }
    const parsed = new URL(url, "http://stub.invalid");
    const path = parsed.pathname.slice("/v1".length);
    const method = input instanceof Request ? input.method : "GET";
    const raw = input instanceof Request ? await input.text() : "";
    calls.push({ path, method, body: raw === "" ? null : JSON.parse(raw) });
    // Keyed by METHOD AND PATH. Keying on the path alone let a scripted GET
    // answer the PUT beside it, so a write this screen was refused looked to
    // the test like one the server accepted.
    const handler = handlers[`${method} ${path}`];
    if (!handler) {
      // A route nobody scripted answers 503 rather than something plausible,
      // so a screen reaching for one fails here instead of rendering an error
      // card that looks like the server's.
      return json({ code: "unavailable" }, 503);
    }
    return json(handler(raw === "" ? null : JSON.parse(raw)));
  };
  return { calls, fetchStub };
}

/** The three reads every rendered screen makes, answered with nothing. */
const QUIET = {
  "GET /ext/openchannel/endpoint": () => ({ opened: false }),
  "GET /ext/openchannel/inbound": () => ({ entries: [] }),
  "GET /ext/openchannel/outbound": () => ({ attempts: [] }),
};

function renderScreen() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <OpenchannelScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["en-GB"],
    configurable: true,
  });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the openchannel screen", () => {
  it("names the page in the one level-1 heading a unit screen owns", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, QUIET);
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const h1 = await screen.findByRole("heading", { level: 1 });
    expect(h1.textContent).toBe("Open channel");
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
  });

  // Not having opened an endpoint is the ordinary first state of this screen,
  // and it is a state rather than a failure.
  it("says no endpoint is open, and offers to open one", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, QUIET);
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(await screen.findByText("No endpoint open")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Open my endpoint" }),
    ).toBeTruthy();
    // Nothing to mint against, so no control that would 404 on the way.
    expect(
      screen.queryByRole("button", { name: "Mint a signing secret" }),
    ).toBeNull();
  });

  it("opens the caller's own endpoint, naming nobody", async () => {
    let opened = false;
    const { calls, fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () =>
        opened ? OPENED : { opened: false },
      "PUT /ext/openchannel/endpoint": () => {
        opened = true;
        return ENDPOINT;
      },
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Open my endpoint" }),
    );

    await waitFor(() => {
      const put = calls.find(
        (call) =>
          call.path === "/ext/openchannel/endpoint" && call.method === "PUT",
      );
      // The empty object is the whole request: there is no member to name,
      // because the endpoint this opens is always the caller's own.
      expect(put?.body).toEqual({});
    });
    expect(await screen.findByText("Accepting")).toBeTruthy();
  });

  // The address is what a person hands to whoever configures the sender, so it
  // has to be complete — and it has to say, where it is shown, that it is not
  // the thing that admits a request.
  it("shows the full inbound address and says it is not a credential", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const address = await screen.findByTestId("openchannel-inbound-url");
    expect(address.textContent).toBe(
      `${globalThis.location.origin}/webhooks/ext/openchannel/receive/k7m2q9x4`,
    );
    expect(screen.getByText(/not a credential/)).toBeTruthy();
  });

  // THE ASSERTION THIS FILE EXISTS FOR. A `curl` that does not verify is worse
  // than none: the person who pastes it is refused by the same opaque 401 a
  // forged request gets, and learns that the connector is broken rather than
  // that the example is. Each clause below is the verifier's own rule —
  // HMAC-SHA256 over `<unix seconds>.<nonce>.<body>`, hex, under the `sha256=`
  // prefix the comparison is made with, sent under the three published header
  // names.
  it("offers a curl that carries what the verifier checks", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const recipe = (await screen.findByTestId("openchannel-curl")).textContent;
    expect(recipe).toContain(
      `${globalThis.location.origin}/webhooks/ext/openchannel/receive/k7m2q9x4`,
    );
    expect(recipe).toContain('-H "X-Margince-Timestamp: $TS"');
    expect(recipe).toContain('-H "X-Margince-Nonce: $NONCE"');
    expect(recipe).toContain('-H "X-Margince-Signature: sha256=$SIG"');
    // The signed material, in the order and with the separators the verifier
    // concatenates them in — and via printf, because echo appends a newline
    // and the MAC covers the bytes it was given.
    expect(recipe).toContain(`printf '%s.%s.%s' "$TS" "$NONCE" "$BODY"`);
    expect(recipe).toContain(`openssl dgst -sha256 -hmac "$SECRET"`);
    // The body is posted verbatim: --data-raw does not reinterpret it, and
    // one edited byte is the difference between a landed message and an
    // opaque refusal.
    expect(recipe).toContain(`--data-raw "$BODY"`);
    // A document the drain can turn into a timeline entry — `message_id` is
    // the one member the record builder refuses a body for missing.
    expect(recipe).toContain(`"message_id":"demo-1"`);
  });

  // The secret exists on a screen exactly once, and the sentence saying so is
  // beside it: a reader who has to hover to learn it has already moved on.
  it("shows a minted secret once, and says it is the only time", async () => {
    const { calls, fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "POST /ext/openchannel/endpoint/secret": () => ({
        signing_secret: "b1946ac92492d2347c6235b4d2611184",
        endpoint: ENDPOINT,
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Mint a signing secret" }),
    );

    const shown = await screen.findByTestId("openchannel-signing-secret");
    expect(shown.textContent).toBe("b1946ac92492d2347c6235b4d2611184");
    expect(screen.getByText(/only time this secret is shown/)).toBeTruthy();
    const mint = calls.find(
      (call) => call.path === "/ext/openchannel/endpoint/secret",
    );
    expect(mint?.method).toBe("POST");
    expect(mint?.body).toEqual({});
  });

  // The read that follows a mint carries no secret, and the screen must not
  // hand one back to a body that carried it anyway.
  it("never renders a secret the endpoint read carried", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => ({
        opened: true,
        endpoint: { ...ENDPOINT, signing_secret: "leaked_by_the_server" },
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    await screen.findByText("Accepting");
    expect(screen.queryByText(/leaked_by_the_server/)).toBeNull();
  });

  it("registers the address this connector talks back to", async () => {
    const { calls, fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "PUT /ext/openchannel/endpoint/url": () => ({
        ...ENDPOINT,
        url: "https://example.com/hooks/crm",
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Where this connector talks back to"),
      "https://example.com/hooks/crm",
    );
    await user.click(screen.getByRole("button", { name: "Register address" }));

    await waitFor(() => {
      const put = calls.find(
        (call) => call.path === "/ext/openchannel/endpoint/url",
      );
      expect(put?.method).toBe("PUT");
      expect(put?.body).toEqual({ url: "https://example.com/hooks/crm" });
    });
  });

  // The state to move to travels as a variable rather than being read off the
  // render the handler closed over — otherwise a poll that moved the endpoint
  // under an open screen would have this button resume what was just paused.
  it("pauses an accepting endpoint", async () => {
    const { calls, fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "PUT /ext/openchannel/endpoint/enabled": () => ({
        ...ENDPOINT,
        enabled: false,
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Pause" }));

    await waitFor(() => {
      const put = calls.find(
        (call) => call.path === "/ext/openchannel/endpoint/enabled",
      );
      expect(put?.body).toEqual({ enabled: false });
    });
  });

  it("resumes a paused endpoint", async () => {
    const { calls, fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => ({
        opened: true,
        endpoint: { ...ENDPOINT, enabled: false },
      }),
      "PUT /ext/openchannel/endpoint/enabled": () => ENDPOINT,
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    expect(await screen.findByText("Paused")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Resume" }));

    await waitFor(() => {
      const put = calls.find(
        (call) => call.path === "/ext/openchannel/endpoint/enabled",
      );
      expect(put?.body).toEqual({ enabled: true });
    });
  });

  it("lists what arrived, and why a stopped request stopped", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "GET /ext/openchannel/inbound": () => ({
        entries: [
          INBOUND_ROW,
          {
            ...INBOUND_ROW,
            id: "1a2b3c4d-0000-4000-8000-000000000003",
            state: "failed",
            attempts: 3,
            last_error_class: "payload_unusable",
          },
        ],
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(await screen.findByText("On the timeline")).toBeTruthy();
    expect(screen.getByText("Stopped")).toBeTruthy();
    // The class in this connector's own words, never a remote party's prose.
    expect(
      screen.getByText(/posted a shape this connector does not accept/),
    ).toBeTruthy();
    // Both rows carry the same payload size, and both are drawn: a listing
    // that collapsed them would hide a redelivery.
    expect(screen.getAllByText("168")).toHaveLength(2);
  });

  // The contract publishes three inbound states and the table writes a fourth
  // (`withdrawn`), so a screen that trusted the enum would put a raw column
  // value in front of a reader.
  it("words a state outside the published enum instead of echoing it", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "GET /ext/openchannel/inbound": () => ({
        entries: [{ ...INBOUND_ROW, state: "withdrawn" }],
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(await screen.findByText("Withdrawn")).toBeTruthy();
    expect(screen.queryByText("withdrawn")).toBeNull();
  });

  it("lists what left", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "GET /ext/openchannel/outbound": () => ({
        attempts: [
          {
            ...OUTBOUND_ROW,
            outcome: "refused",
            error_class: "delivery_refused",
          },
        ],
      }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(await screen.findByText("Refused")).toBeTruthy();
    expect(screen.getByText(/answered and declined/)).toBeTruthy();
  });

  it("says so when nothing has arrived and nothing has left", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(
      await screen.findByText("Nothing has arrived on your endpoint yet."),
    ).toBeTruthy();
    expect(
      screen.getByText("Nothing has been sent from your endpoint yet."),
    ).toBeTruthy();
  });

  // A seat that may look and not touch gets NO controls — absent, not
  // disabled. A control that leads to a 403 is worse than one that is not
  // there, and a disabled one says "this is yours, later".
  it("offers a read-only seat the lists and no controls at all", async () => {
    const { fetchStub } = stubTransport(READ_ONLY_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
      "GET /ext/openchannel/inbound": () => ({ entries: [INBOUND_ROW] }),
      "GET /ext/openchannel/outbound": () => ({ attempts: [OUTBOUND_ROW] }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    // It reads everything a full seat does…
    expect(await screen.findByText("On the timeline")).toBeTruthy();
    expect(screen.getByText("Sent")).toBeTruthy();
    expect(screen.getByTestId("openchannel-inbound-url")).toBeTruthy();
    // …and can change nothing. No button on the page at all, which is a
    // stronger claim than naming the four and is the one that survives a fifth
    // control being added.
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(
      screen.queryByLabelText("Where this connector talks back to"),
    ).toBeNull();
  });

  // An ungranted seat is told so, and — the part that matters — no request is
  // fired: a refused read on a fifteen-second timer is a failing screen where
  // the honest answer is "you were not granted this".
  it("tells an ungranted seat, and asks the server nothing", async () => {
    const { calls, fetchStub } = stubTransport(NO_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => OPENED,
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    expect(
      await screen.findByText(/not been granted access to this connector/),
    ).toBeTruthy();
    expect(
      await screen.findByText(/not been granted access to what arrives here/),
    ).toBeTruthy();
    expect(calls.filter((call) => call.path.startsWith("/ext/"))).toHaveLength(
      0,
    );
  });

  // A body this screen cannot read is an error, not "no endpoint" — the second
  // invites a member to open one over an edge senders are already configured
  // against.
  it("does not read an unreadable answer as an unopened endpoint", async () => {
    const { fetchStub } = stubTransport(FULL_GRANT, {
      ...QUIET,
      "GET /ext/openchannel/endpoint": () => ({ something_else: true }),
    });
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    // The read failure announces itself, and nothing on the page claims the
    // member has no endpoint.
    const alert = await screen.findByRole("alert");
    expect(within(alert).getByText("Couldn't load this view.")).toBeTruthy();
    expect(screen.queryByText("No endpoint open")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Open my endpoint" }),
    ).toBeNull();
  });

  // A failed write is ANNOUNCED, not merely rendered: it appears after the
  // press that caused it, so a reader who has moved off the button otherwise
  // hears nothing and is left believing the endpoint opened.
  it("announces a failed open as an alert", async () => {
    // PUT is deliberately unscripted, so the stub answers 503 — a real refusal
    // shape rather than a thrown fetch, which no server produces.
    const { fetchStub } = stubTransport(FULL_GRANT, QUIET);
    vi.stubGlobal("fetch", vi.fn(fetchStub));

    renderScreen();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Open my endpoint" }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "The endpoint may not have been opened. Check the state above before trying again.",
    );
  });
});
