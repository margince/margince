/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { locationDouble } from "../../testing/locationdouble";
import { meFixture } from "../../testing/mefixture";
import { type RouteMap, stubWithSession } from "../../testing/story-utils";
import { ConnectorsCard } from "./connectors";

// The connected-inboxes card makes the onboarding promise ("disconnect in one
// click", "manage in Settings") real. It renders server facts only, and a
// disconnect is confirmed-first before it stops capture.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const gmailConnected: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  last_synced_at: "2026-07-23T09:30:00Z",
  last_success_at: "2026-07-23T09:30:00Z",
  // A finished backfill. Mounting BackfillPanel below the row must not fire an
  // extra request, because the panel seeds from this embedded snapshot. "none"
  // would auto-fire the setup screen's scope preview against an unstubbed
  // route. "done" is the inert terminal state for a connection these fixtures
  // do not otherwise care about.
  backfill: { state: "done" },
};

const gmailStale: CaptureConnection = {
  ...gmailConnected,
  status: "reauth_required",
};

// A mailbox connected before Margince asked for the send scope: healthy,
// capturing, and permanently unable to send until it is reconnected — Google
// will not widen an existing refresh token.
const gmailNoSendGrant: CaptureConnection = {
  ...gmailConnected,
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
};

const imapConnected: CaptureConnection = {
  ...gmailConnected,
  id: "018f3a1b-0000-7000-8000-0000000000c9",
  provider: "imap",
  can_backfill: false,
  scopes: [],
};

// The calendar half of the same Google grant. Its account_label is the member's
// own email address — the vendor gives the same identity for both connections —
// so nothing on the row but its NAME says this is not their mailbox.
const gcalConnected: CaptureConnection = {
  ...gmailConnected,
  id: "018f3a1b-0000-7000-8000-0000000000ca",
  provider: "gcal",
  can_backfill: false,
  scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type StubOpts = {
  /** Fail the /connectors GET with this status (load-error path). */
  listStatus?: number;
  /** The connect (reconnect) POST response, or an error status. */
  connect?: { authorize_url?: string } | { status: number };
  /** The session GET /me answers; unset, an admin holding no object grants. */
  me?: components["schemas"]["MeResponse"];
};

function stubApi(connections: CaptureConnection[], opts: StubOpts = {}) {
  const calls: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push(request);
      const path = new URL(request.url).pathname;
      if (path.endsWith("/connectors") && request.method === "GET") {
        if (opts.listStatus) {
          return jsonResponse({ detail: "boom" }, opts.listStatus);
        }
        return jsonResponse({ data: connections });
      }
      if (path.endsWith("/me") && request.method === "GET") {
        return jsonResponse(opts.me ?? meFixture({}));
      }
      if (path.endsWith("/capture/settings") && request.method === "GET") {
        return jsonResponse({
          auto_enrich: true,
          mail_sharing: true,
          shared_posture_allowed: false,
          signature_enrich: true,
        });
      }
      if (path.endsWith("/connect") && request.method === "POST") {
        const c = opts.connect ?? {
          authorize_url: "https://accounts.google/x",
        };
        if ("status" in c) {
          const body =
            c.status === 501
              ? { code: "not_implemented", detail: "capture not wired" }
              : { detail: "connect failed" };
          return jsonResponse(body, c.status);
        }
        return jsonResponse(c);
      }
      if (path.endsWith("/disconnect") && request.method === "POST") {
        return new Response(null, { status: 204 });
      }
      throw new Error(`unstubbed: ${request.method} ${path}`);
    }),
  );
  return calls;
}

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The mail-half cases are about no grant: the session is an admin holding none.
function stubMailRoutes(routes: RouteMap) {
  stubWithSession(routes, {});
}

function requestsTo(calls: Request[], suffix: string, method: string) {
  return calls.filter(
    (r) => new URL(r.url).pathname.endsWith(suffix) && r.method === method,
  );
}

// Adding an account is the card's create verb, so it lives in the panel
// header and opens the connect dialog. Every claim about a pick therefore opens
// the dialog first and scopes its queries to it.
async function openAddDialog() {
  await userEvent.click(
    await screen.findByRole("button", { name: "Add account" }),
  );
  return screen.getByRole("dialog", { name: "Connect mail and calendar" });
}

/** Opens an account's overflow menu and chooses one of its verbs. */
async function openAccountVerb(account: string, verb: string) {
  await userEvent.click(
    await screen.findByRole("button", { name: `More actions for ${account}` }),
  );
  await userEvent.click(await screen.findByRole("button", { name: verb }));
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

describe("the connected-inboxes card", () => {
  it("lists a live connection with its status and last-synced time", async () => {
    stubApi([gmailConnected]);
    render(<ConnectorsCard />);
    expect(
      await screen.findByRole("heading", { name: "Google" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Connected")).toBeInTheDocument();
    const newMail = screen.getByTestId("connector-gmail");
    expect(within(newMail).getByText("Up to date")).toBeInTheDocument();
    expect(within(newMail).getByText(/Last synced/)).toBeInTheDocument();
  });

  // The workspace's bot is company wiring and lives on Integrations, and the
  // emailed-links address is reported on System health: neither is a mailbox.
  it("carries neither the Telegram bot nor the public address", async () => {
    const calls = stubApi([gmailConnected]);
    render(<ConnectorsCard />);
    await screen.findByRole("heading", { name: "Mail and calendar" });
    expect(screen.queryByRole("heading", { name: "Telegram bot" })).toBeNull();
    expect(screen.queryByTestId("public-origin")).toBeNull();
    expect(requestsTo(calls, "/channel-connections", "GET")).toHaveLength(0);
  });

  // The history import belongs to the mailbox it imports for, as one of the
  // services its account lists. A row that only offers the import counts
  // nothing. The scope preview is a provider round trip, so it waits for the
  // dialog the reader opens.
  it("lists past email under the mailbox's own account without counting it", async () => {
    const calls = stubApi([{ ...gmailConnected, backfill: { state: "none" } }]);
    render(<ConnectorsCard />);
    const pastEmail = await screen.findByTestId("connector-gmail-past-email");
    expect(within(pastEmail).getByText("Not imported")).toBeInTheDocument();
    expect(
      within(pastEmail).getByRole("button", { name: "Import past email" }),
    ).toBeInTheDocument();
    expect(requestsTo(calls, "/backfill/preview", "POST")).toHaveLength(0);
  });

  // A calendar is not a mailbox, and the three mail-shaped rows do not belong to
  // one. A history-import card under a Google Calendar once answered "this
  // mailbox type can't be backfilled". A member read that as a refusal to import
  // their mail. The account label is their own email address on both
  // connections, so the row gave no reason to think otherwise.
  it("draws no mail-shaped row against a calendar connection", async () => {
    stubApi([gcalConnected]);
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-gcal");
    expect(screen.queryByTestId("connector-gmail-past-email")).toBeNull();
    expect(screen.queryByTestId("connector-gcal-mail-posture")).toBeNull();
    expect(screen.queryByText(/Import your mail history/)).toBeNull();
  });

  // The control: the same three rows against a mailbox. Without it a card that
  // had stopped rendering rows at all would satisfy the case above.
  it("draws the mail-shaped rows against a mailbox", async () => {
    stubApi([gmailConnected]);
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-gmail");
    expect(screen.getByTestId("connector-gmail-past-email")).toBeTruthy();
    expect(screen.getByTestId("connector-gmail-mail-posture")).toBeTruthy();
  });

  // The connector says whether old mail can come in, so a mailbox whose
  // connector cannot import offers no import that would only be refused.
  it("offers no past email where the connector cannot import old mail", async () => {
    stubApi([{ ...gmailConnected, can_backfill: false }]);
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-gmail");
    expect(screen.queryByTestId("connector-gmail-past-email")).toBeNull();
  });

  // And the tag row stays. A meeting brings in contacts as a message does, so
  // which source found them is the same question on either kind.
  it("keeps the context tag row on a calendar connection", async () => {
    stubApi([
      {
        ...gcalConnected,
        context_tag: {
          id: "t-cal",
          name: "From the calendar",
          archived: false,
        },
      },
    ]);
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-gcal");
    expect(screen.getByTestId("connector-gcal-context-tag")).toBeTruthy();
  });

  it("offers every account from the dialog once one account is connected", async () => {
    stubApi([{ ...gmailConnected, provider: "imap" }]);
    render(<ConnectorsCard />);
    const dialog = await openAddDialog();
    for (const account of ["Google", "Microsoft 365"]) {
      expect(
        within(dialog).getByRole("radio", { name: new RegExp(`^${account}`) }),
      ).toBeEnabled();
    }
  });

  it("opens the inline IMAP form from the dialog instead of bouncing to onboarding", async () => {
    stubApi([gmailConnected]);
    render(<ConnectorsCard />);
    const dialog = await openAddDialog();
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /^Another mail host/ }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    // The dialog gives way rather than stacking behind the form: two overlays
    // deep, Escape and the focus restore both answer to the wrong layer.
    expect(
      await screen.findByRole("dialog", { name: "Connect IMAP mailbox" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("dialog", { name: "Connect mail and calendar" }),
    ).toBeNull();
  });

  it("offers reconnect only for a connection that needs re-auth", async () => {
    stubApi([gmailStale]);
    render(<ConnectorsCard />);
    expect(await screen.findByText("Needs reconnect")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Reconnect/ })).toBeTruthy();
  });

  it("keeps one primary Reconnect when two accounts need one", async () => {
    stubApi([
      gmailStale,
      {
        ...gmailStale,
        id: "018f3a1b-0000-7000-8000-0000000000cb",
        provider: "graph",
        scopes: [],
      },
    ]);
    render(<ConnectorsCard />);
    await screen.findByRole("heading", { name: "Microsoft 365" });
    const [first, second] = screen.getAllByRole("button", {
      name: "Reconnect",
    });
    expect(first).toHaveClass("btn-primary");
    expect(second).not.toHaveClass("btn-primary");
  });

  it("says a Gmail mailbox cannot send before the rep discovers it at send time", async () => {
    stubApi([gmailNoSendGrant]);
    render(<ConnectorsCard />);
    expect(await screen.findByText("Capture only, no sending")).toBeTruthy();
    expect(
      screen.getByText(/Reconnect this mailbox to send from it/),
    ).toBeTruthy();
    // The prompt is only actionable if reconnecting is one click from here.
    expect(screen.getByRole("button", { name: /Reconnect/ })).toBeTruthy();
  });

  it("stays quiet about sending once the send scope is granted", async () => {
    stubApi([gmailConnected]);
    render(<ConnectorsCard />);
    expect(await screen.findByText("Up to date")).toBeTruthy();
    expect(screen.queryByText("Capture only, no sending")).toBeNull();
    expect(screen.queryByRole("button", { name: /Reconnect/ })).toBeNull();
  });

  it("never claims a non-Gmail mailbox cannot send — its scopes are not Google's", async () => {
    stubApi([imapConnected]);
    render(<ConnectorsCard />);
    expect(await screen.findByText("IMAP mailbox")).toBeTruthy();
    expect(screen.queryByText("Capture only, no sending")).toBeNull();
  });

  it("shows an honest waiting line for a connection that has never synced", async () => {
    stubApi([
      { ...gmailConnected, last_synced_at: null, last_success_at: null },
    ]);
    render(<ConnectorsCard />);
    expect(await screen.findByText(/No successful sync recorded/)).toBeTruthy();
  });

  it("keeps the successful sync time unknown for an older server", async () => {
    stubApi([{ ...gmailConnected, last_success_at: undefined }]);
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/Successful sync time unavailable/),
    ).toBeTruthy();
    expect(screen.queryByText("No successful sync recorded")).toBeNull();
  });

  it("surfaces a load failure without crashing the card", async () => {
    stubApi([], { listStatus: 500 });
    render(<ConnectorsCard />);
    // The card's own claim as the heading, the server's cause under it: both,
    // because the heading alone says nothing about why.
    expect(await screen.findByText(/Could not load this view/)).toBeTruthy();
    expect(await screen.findByText(/boom/)).toBeTruthy();
  });

  it("reconnect re-mints the consent URL and redirects", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", locationDouble({ assign }));
    const calls = stubApi([gmailStale], {
      connect: { authorize_url: "https://accounts.google/consent" },
    });
    render(<ConnectorsCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /Reconnect/ }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/connect", "POST").length).toBe(1),
    );
    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://accounts.google/consent"),
    );
  });

  it("sends return_to=settings on reconnect so consent lands back on Settings", async () => {
    vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
    const calls = stubApi([gmailStale], {
      connect: { authorize_url: "https://accounts.google/consent" },
    });
    render(<ConnectorsCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /Reconnect/ }),
    );
    const connectRequests = await waitFor(() => {
      const requests = requestsTo(calls, "/connect", "POST");
      expect(requests.length).toBe(1);
      return requests;
    });
    const body = await connectRequests[0].clone().json();
    expect(body).toEqual({ return_to: "settings" });
  });

  it("offers the inline IMAP form to reconnect an imap connection instead of an OAuth reconnect", async () => {
    stubApi([{ ...gmailStale, provider: "imap" }]);
    render(<ConnectorsCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /Reconnect/ }),
    );
    expect(await screen.findByText("Connect IMAP mailbox")).toBeTruthy();
  });

  it("surfaces a failed reconnect instead of redirecting", async () => {
    const calls = stubApi([gmailStale], { connect: { status: 502 } });
    render(<ConnectorsCard />);
    await userEvent.click(
      await screen.findByRole("button", { name: /Reconnect/ }),
    );
    await waitFor(() =>
      expect(requestsTo(calls, "/connect", "POST").length).toBe(1),
    );
    expect(await screen.findByText(/connect failed/)).toBeTruthy();
    // Announced, and attached to the account whose Reconnect produced it, at the
    // row's full width. Never in a band of its own under an unrelated heading,
    // and never squeezed into the service's verb column.
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/connect failed/);
    const service = screen.getByTestId("connector-gmail");
    expect(service.contains(alert)).toBe(false);
    expect(service.closest(".provider-row")?.contains(alert)).toBe(true);
  });
});

// The richer per-row health line (account_label, next_sync_due_at,
// watch_expires_at, the error-class sentence) and the 501 calm state, all
// exercised through the real story-utils route-map shape.
describe("the connected-inboxes card's richer health line", () => {
  it("shows the account label beside the provider name", async () => {
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({
          data: [{ ...gmailConnected, account_label: "lars@example.de" }],
        }),
    });
    render(<ConnectorsCard />);
    expect(await screen.findByText("lars@example.de")).toBeTruthy();
  });

  // How the sync works underneath (push or poll, the next check) is not a fact a
  // member acts on. The default view leaves it out.
  it("keeps the sync mechanics out of the default view", async () => {
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              ...gmailConnected,
              watch_expires_at: "2026-08-01T00:00:00Z",
              next_sync_due_at: "2026-07-23T09:35:00Z",
            },
          ],
        }),
    });
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-gmail");
    expect(screen.queryByText(/push renewal/i)).toBeNull();
    expect(screen.queryByText(/polled/i)).toBeNull();
    expect(screen.queryByText(/next check/i)).toBeNull();
  });

  it("renders the error-class sentence for a reauth_required connection", async () => {
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({
          data: [{ ...gmailStale, last_sync_error_class: "auth" }],
        }),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/rejected the stored credentials/i),
    ).toBeTruthy();
  });

  it("renders the 501 not-configured response as a calm state, not an error", async () => {
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ code: "not_implemented" }, 501),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/is not configured on this installation/i),
    ).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByText(/couldn't load/i)).toBeNull();
  });

  it("shows the updated disconnect copy naming credential deletion and Google's own access list", async () => {
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [gmailConnected] }),
    });
    render(<ConnectorsCard />);
    await openAccountVerb("Google", "Disconnect");
    expect(
      await screen.findByText(/deletes the stored credential/i),
    ).toBeTruthy();
    expect(screen.getByText(/Google may still list Margince/i)).toBeTruthy();
  });

  it("omits the vendor-access note for an IMAP disconnect (no upstream grant)", async () => {
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({ data: [{ ...gmailConnected, provider: "imap" }] }),
    });
    render(<ConnectorsCard />);
    await openAccountVerb("IMAP mailbox", "Disconnect");
    expect(
      await screen.findByText(/deletes the stored credential/i),
    ).toBeTruthy();
    expect(screen.queryByText(/Google may still list Margince/i)).toBeNull();
  });
});

// The OAuth return outcome: the backend lands the callback on
// #/settings/connections/{outcome}. The route parses to
// {screen:"settings", id:"connections", id2:<outcome>}. The card renders a
// dismissible inline note from that segment, never a claim the server has not
// confirmed.
describe("the OAuth return outcome", () => {
  // An import asked for at connect that the server held for the reader rides
  // the landing as ?import=. It rides the calendar's landing too when the
  // consent was chained, and lands on the account it was asked for.
  it("offers the held import on the account the landing names", async () => {
    globalThis.location.hash = "#/settings/connections/ok/gcal?import=12m";
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            { ...gmailConnected, backfill: { state: "none" } },
            gcalConnected,
          ],
        }),
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "12m",
          estimated_messages: 8200,
          computed_at: "2026-07-23T10:00:00Z",
        }),
    });
    render(<ConnectorsCard />);
    const pastEmail = await screen.findByTestId("connector-gmail-past-email");
    expect(
      await within(pastEmail).findByText("8,200 emails in that time."),
    ).toBeInTheDocument();
    expect(
      within(pastEmail).getByRole("button", { name: "Start import" }),
    ).toBeInTheDocument();
  });

  it("leaves another vendor's account alone when the landing holds an import", async () => {
    globalThis.location.hash = "#/settings/connections/ok/graph?import=12m";
    stubMailRoutes({
      "GET /connectors": () =>
        jsonResponse({
          data: [{ ...gmailConnected, backfill: { state: "none" } }],
        }),
    });
    render(<ConnectorsCard />);
    const pastEmail = await screen.findByTestId("connector-gmail-past-email");
    expect(within(pastEmail).getByText("Not imported")).toBeInTheDocument();
    expect(
      within(pastEmail).queryByRole("button", { name: "Start import" }),
    ).toBeNull();
  });

  it("renders an honest denial note when the user declined access", async () => {
    globalThis.location.hash = "#/settings/connections/denied";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/Access was declined, so nothing/i),
    ).toBeTruthy();
    expect(screen.queryByText(/connection could not be completed/i)).toBeNull();
  });

  it("renders an honest failure note when the connection could not complete", async () => {
    globalThis.location.hash = "#/settings/connections/error";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/connection could not be completed/i),
    ).toBeTruthy();
    expect(screen.queryByText(/Access was declined, so nothing/i)).toBeNull();
  });

  // A permanent failure must not tell the reader to try again: the provider's
  // API is not enabled for this deployment, and only an administrator can
  // change that.
  it("names the remedy when the provider's API is not enabled here", async () => {
    globalThis.location.hash = "#/settings/connections/misconfigured";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/administrator must enable it/i),
    ).toBeTruthy();
    expect(screen.queryByText(/connection could not be completed/i)).toBeNull();
  });

  it("tells the reader to accept every permission when the provider declined", async () => {
    globalThis.location.hash = "#/settings/connections/rejected";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    expect(
      await screen.findByText(/Accept every requested permission/i),
    ).toBeTruthy();
    // The generic "couldn't be completed — please try again" must not also show.
    expect(screen.queryByText(/connection could not be completed/i)).toBeNull();
  });

  it("renders a brief success note on ok — never an error", async () => {
    globalThis.location.hash = "#/settings/connections/ok";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [gmailConnected] }),
    });
    render(<ConnectorsCard />);
    expect(await screen.findByText(/mailbox is capturing/i)).toBeTruthy();
    expect(screen.queryByText(/connection could not be completed/i)).toBeNull();
    expect(screen.queryByText(/Access was declined, so nothing/i)).toBeNull();
  });

  it("renders no outcome note when the route carries none", async () => {
    globalThis.location.hash = "#/settings/connections";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    await screen.findByTestId("connect-tile-google");
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("dismisses the note and clears it", async () => {
    globalThis.location.hash = "#/settings/connections/denied";
    stubMailRoutes({
      "GET /connectors": () => jsonResponse({ data: [] }),
    });
    render(<ConnectorsCard />);
    await screen.findByText(/Access was declined, so nothing/i);
    await userEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(screen.queryByText(/Access was declined, so nothing/i)).toBeNull();
  });
});

// "Add account" opens the connect dialog over the roster it adds to.
// connect-flow.test.tsx pins what the dialog sends for each answer. These cases
// pin what the card hands it: the roster, and the way out to the IMAP form.
describe("add an account", () => {
  it("offers what is left of a half-connected account, and refuses a full one", async () => {
    stubApi([
      gmailConnected,
      { ...gmailConnected, id: "c2", provider: "gcal" },
    ]);
    render(<ConnectorsCard />);
    const dialog = await openAddDialog();
    const google = within(dialog).getByRole("radio", { name: /^Google/ });
    expect(google).toBeDisabled();
    expect(google.closest("label")?.textContent).toContain(
      "Already connected.",
    );
    expect(
      within(dialog).getByRole("radio", { name: /^Microsoft 365/ }),
    ).toBeEnabled();
  });

  it("connects the calendar Gmail is missing and hands the browser over", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", locationDouble({ assign }));
    const calls = stubApi([gmailConnected], {
      connect: { authorize_url: "https://accounts.google/cal" },
    });
    render(<ConnectorsCard />);
    const dialog = await openAddDialog();
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /^Google/ }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue to Google" }),
    );
    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://accounts.google/cal"),
    );
    expect(requestsTo(calls, "/connectors/gcal/connect", "POST")).toHaveLength(
      1,
    );
  });

  it("reports a refused connect inside the dialog whose button produced it", async () => {
    stubApi([gmailConnected], { connect: { status: 502 } });
    render(<ConnectorsCard />);
    const dialog = await openAddDialog();
    await userEvent.click(
      within(dialog).getByRole("radio", { name: /^Microsoft 365/ }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue to Microsoft 365" }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toMatch(/connect failed/);
    // In the dialog the press happened in. Not in a band of its own on the card
    // behind it, where nothing says which press it answers.
    expect(dialog.contains(alert)).toBe(true);
  });

  it("withdraws the header verb when every account is fully connected", async () => {
    stubApi([
      gmailConnected,
      { ...gmailConnected, id: "c2", provider: "gcal" },
      { ...gmailConnected, id: "c3", provider: "graph" },
      { ...gmailConnected, id: "c4", provider: "graphcal" },
      { ...gmailConnected, id: "c5", provider: "imap" },
    ]);
    render(<ConnectorsCard />);
    await screen.findByTestId("connector-graphcal"); // a roster service
    expect(screen.queryByRole("button", { name: "Add account" })).toBeNull();
  });
});

it("offers reconnect for a healthy read-only calendar", async () => {
  stubApi([gcalConnected]);
  render(<ConnectorsCard />);
  expect(await screen.findByText("Read-only calendar")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Reconnect" })).toBeEnabled();
});
