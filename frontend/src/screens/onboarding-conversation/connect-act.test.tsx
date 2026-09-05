/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
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
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { installFetchStub, jsonResponse, meRoute } from "../story-utils";
import { ConnectAct } from "./connect-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The Microsoft chip must open the SAME live OAuth panel as Google (no more
// disabled "Soon" badge), and the post-consent return view no longer depends
// on which chip was open when the redirect left — it reads the roster fresh.
//
// The act also owns the finish, and only the surface's own Continue reaches
// it: the backread that follows a confirmed connection can be left running or
// declined, and either way its dialog closes back onto the cards — LinkedIn
// still waiting — rather than carrying the reader out of the step. The step
// completes exactly once, from the surface: CONNECT_DONE is the only event the
// mail path dispatches and a second one would move the machine out from under
// the wizard.

// Every stub in this file routes the session probe, because ConnectAct mounts
// capability-aware chrome and story-utils refuses to guess a session: an
// unrouted GET /me answers 501, every grant fails closed, and the provider
// cards stay disabled for a reason no assertion here is about. The grants are
// empty on purpose — this act is onboarding, before any of them are held.
function stubWithSession(routes: Parameters<typeof installFetchStub>[0]) {
  installFetchStub({ "GET /me": meRoute({}), ...routes });
}

function renderConnectAct(
  outcome?: string,
  linkedinStatus: ConversationState["linkedinStatus"] = "pending",
  phase: ConversationState["phase"] = "cn.consent",
  returningProvider?: string,
) {
  const state: ConversationState = {
    ...initialConversationState,
    act: "connect",
    phase,
    linkedinStatus,
  };
  const dispatch = vi.fn();
  const persist = vi.fn(async () => true);
  const view = render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectAct
          state={state}
          dispatch={dispatch}
          persist={persist}
          outcome={outcome}
          returningProvider={returningProvider}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, dispatch, persist };
}

const rosterWith = (backfill: Record<string, unknown>) => () =>
  jsonResponse({
    data: [
      {
        id: "g1",
        provider: "gmail",
        status: "connected",
        scopes: ["read"],
        backfill,
      },
    ],
  });

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});
// Explicit, because auto-cleanup only runs with vitest globals enabled and
// this suite does not use them: without it each test inherits the previous
// render's DOM and every chip query finds several matches.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // The OAuth-attempt mark lives in sessionStorage (see
  // onboarding-connect-panels.tsx) — jsdom shares it across tests in this
  // same process, so a mark one test writes must not leak into the next.
  sessionStorage.clear();
});

it("offers Microsoft as a live card and opens its dialog", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
  renderConnectAct();
  // The card names the provider AND what connecting it grants, so the
  // accessible name is the whole card, not the brand alone.
  const ms = screen.getByRole("button", { name: /Microsoft/ });
  // Idle until the roster confirms nothing is connected yet — the fetch
  // above resolves asynchronously, so the card starts disabled.
  await waitFor(() => expect(ms).not.toBeDisabled(), { timeout: 3000 });
  await userEvent.click(ms);
  // The ask opens as its OWN dialog on the surface, never an inline panel
  // growing under the card.
  expect(await screen.findByRole("dialog")).toBeTruthy();
  expect(
    await screen.findByRole("button", {
      name: "Allow access to my Microsoft",
    }),
  ).toBeTruthy();
});

// The roster says which providers this installation can actually start a
// connect for, decided by the same predicate the connect endpoint uses. A card
// that opened anyway would send the reader through a dialog to a 501 they can
// do nothing about, so an unconnectable provider stops being a button and says
// what is missing instead.
it("does not offer a provider whose OAuth app is not registered", async () => {
  stubWithSession({
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [
          { provider: "gmail", reason: "ready" },
          { provider: "graph", reason: "app_missing" },
          { provider: "imap", reason: "ready" },
        ],
      }),
  });
  renderConnectAct();

  // Google is untouched: this is one vendor's configuration, not a broken
  // screen, and a reader with a Google mailbox must still get through.
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );
  expect(screen.queryByRole("button", { name: /Microsoft/ })).toBeNull();
  expect(
    screen.getByText(/has not registered its Microsoft app yet/),
  ).toBeTruthy();
  // The one thing the reader can do about it, reachable from here.
  expect(
    screen.getByRole("link", { name: "Set it up in Settings" }),
  ).toBeTruthy();
});

// A stored app whose secret will not open is a DIFFERENT fix from no app at
// all: telling this operator that none exists sends them to register a second
// one they already have.
it("separates an unusable app from a missing one", async () => {
  stubWithSession({
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [{ provider: "graph", reason: "app_unusable" }],
      }),
  });
  renderConnectAct();

  expect(
    await screen.findByText(/Microsoft app cannot be opened right now/),
  ).toBeTruthy();
  expect(screen.queryByText(/has not registered/)).toBeNull();
});

// Nothing in Settings fixes a deployment that does not serve the provider at
// all, so the card offers no link to a form that would have nothing in it.
it("offers no settings link for a provider this deployment does not serve", async () => {
  stubWithSession({
    "GET /connectors": () =>
      jsonResponse({
        data: [],
        providers: [{ provider: "graph", reason: "unsupported" }],
      }),
  });
  renderConnectAct();

  expect(await screen.findByText(/does not serve Microsoft/)).toBeTruthy();
  expect(
    screen.queryByRole("link", { name: "Set it up in Settings" }),
  ).toBeNull();
});

// A card left clickable while the roster is unread would let a reader connect
// a second mailbox before the fetch ever reports the first — the scene's own
// "pick one" rule depends on the roster actually being verified first.
it("withholds every mail provider card until the roster load settles", async () => {
  // A box, not a bare `let`: TS's control-flow narrowing otherwise loses the
  // function type across the callback boundary that assigns it.
  const deferred: { resolve: ((r: Response) => void) | null } = {
    resolve: null,
  };
  stubWithSession({
    "GET /connectors": () =>
      new Promise((resolve) => {
        deferred.resolve = resolve;
      }),
  });
  renderConnectAct();

  for (const name of [/Google/, /Microsoft/, /Any other mailbox/]) {
    expect(screen.getByRole("button", { name })).toBeDisabled();
  }

  deferred.resolve?.(jsonResponse({ data: [] }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );
});

// A roster fetch that failed reports the same "nothing confirmed yet" fact as
// one still loading — actionable cards here would offer to connect a second
// mailbox the failed read simply never got to report.
it("withholds every mail provider card when the roster fetch fails", async () => {
  stubWithSession({
    "GET /connectors": () => jsonResponse({ code: "internal" }, 500),
  });
  renderConnectAct();

  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled(),
  );
  expect(screen.getByRole("button", { name: /Microsoft/ })).toBeDisabled();
  expect(
    screen.getByRole("button", { name: /Any other mailbox/ }),
  ).toBeDisabled();
});

// The mark is what tells a genuine return apart from a stale or bookmarked
// outcome URL (see `attemptedProvider` in connect-act.tsx) — it has to be
// written before the redirect actually leaves, not after.
it("marks this tab's own attempt before the real redirect fires", async () => {
  stubWithSession({
    "GET /connectors": () => jsonResponse({ data: [] }),
    "POST /connectors/graph/connect": () =>
      jsonResponse({ authorize_url: "https://login.microsoftonline/x" }),
  });
  const assign = vi.fn();
  vi.stubGlobal("location", { ...globalThis.location, assign });
  renderConnectAct();
  await userEvent.click(screen.getByRole("button", { name: /Microsoft/ }));
  await userEvent.click(
    screen.getByRole("button", { name: "Allow access to my Microsoft" }),
  );
  await waitFor(() => expect(assign).toHaveBeenCalled());
  expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBe("graph");
});

describe("returning to the dialog a proven attempt left from", () => {
  it("reopens the same dialog, showing the result rather than a fresh ask", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "graph");
    stubWithSession({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              id: "g1",
              provider: "graph",
              status: "connected",
              scopes: ["read"],
              backfill: { state: "done" },
            },
          ],
        }),
    });
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeTruthy();
    // The plain provider name, not the pre-consent "access needed" ask —
    // nothing is being requested any more.
    expect(
      screen.getByRole("heading", { name: "Microsoft" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    // Consumed on read: a reload of this same URL would find no mark.
    expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBeNull();
  });

  it("falls back to the plain inline result when the URL's provider does not match this tab's own attempt", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "gmail");
    stubWithSession({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              id: "g1",
              provider: "graph",
              status: "connected",
              scopes: ["read"],
              backfill: { state: "done" },
            },
          ],
        }),
    });
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("falls back to the plain inline result when no mark is recorded at all — a stale or bookmarked link", async () => {
    stubWithSession({
      "GET /connectors": () =>
        jsonResponse({
          data: [
            {
              id: "g1",
              provider: "graph",
              status: "connected",
              scopes: ["read"],
              backfill: { state: "done" },
            },
          ],
        }),
    });
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

it("renders the provider-agnostic return view on OAuth return", async () => {
  stubWithSession({
    "GET /connectors": () =>
      jsonResponse({
        data: [
          {
            id: "g1",
            provider: "graph",
            status: "connected",
            scopes: ["read"],
            backfill: { state: "done" },
          },
        ],
      }),
  });
  renderConnectAct("ok");
  expect(await screen.findByText("Live and capturing")).toBeTruthy();
});

it("asks how far back to read once the mailbox is confirmed", async () => {
  stubWithSession({
    "GET /connectors": rosterWith({ state: "none" }),
    "POST /connectors/gmail/backfill/preview": () =>
      jsonResponse({
        window: "6m",
        estimated_messages: 900,
        computed_at: "2026-07-31T10:00:00Z",
      }),
  });
  renderConnectAct("ok");
  expect(
    await screen.findByRole("heading", {
      name: "How far back should I read?",
    }),
  ).toBeTruthy();
  // Exactly one history-read decision on the surface — the standalone
  // Settings-style backfill panel is not a second one stacked beside it.
  expect(screen.queryByText("Import your mail history")).toBeNull();
  expect(
    screen.getAllByRole("heading", { name: "How far back should I read?" }),
  ).toHaveLength(1);
});

// Leaving the backread with the read still running puts the reader back on
// the surface, LinkedIn card and all: the step is not left until they press
// its own Continue, which then records the mailbox as connected.
it("closes the backread onto the surface and finishes only from the surface's Continue", async () => {
  stubWithSession({
    "GET /connectors": rosterWith({
      state: "running",
      counts: { messages_scanned: 12 },
    }),
  });
  const { dispatch, persist } = renderConnectAct("ok");

  await userEvent.click(
    await screen.findByRole("button", { name: "Continue while it reads" }),
  );

  // Nothing left the step; the result is gone and the LinkedIn card is
  // reachable, which is the whole point of not leaving.
  await waitFor(() =>
    expect(screen.queryByText("Live and capturing")).toBeNull(),
  );
  expect(dispatch).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: /LinkedIn/ })).not.toBeDisabled();

  await userEvent.click(screen.getByRole("button", { name: "Continue" }));
  await waitFor(() => expect(dispatch).toHaveBeenCalledTimes(1));
  expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" });
  // The mailbox IS connected on this path, so the connect step was not skipped.
  expect(persist).toHaveBeenCalledWith(
    expect.objectContaining({ connectSkipped: false }),
  );
});

it("declining the history read closes the result without starting one or leaving the step", async () => {
  const starts: unknown[] = [];
  stubWithSession({
    "GET /connectors": rosterWith({ state: "none" }),
    "POST /connectors/gmail/backfill/preview": () =>
      jsonResponse({
        window: "6m",
        estimated_messages: 900,
        computed_at: "2026-07-31T10:00:00Z",
      }),
    "POST /connectors/gmail/backfill": (body) => {
      starts.push(body);
      return jsonResponse({ state: "queued" }, 202);
    },
  });
  const { dispatch } = renderConnectAct("ok");

  await userEvent.click(
    await screen.findByRole("button", { name: "Do not read history now" }),
  );

  await waitFor(() =>
    expect(screen.queryByText("Live and capturing")).toBeNull(),
  );
  expect(dispatch).not.toHaveBeenCalled();
  expect(starts).toEqual([]);
});

// The way past a missing mailbox is offered while connecting is still the
// open question — worded for what it is, since LinkedIn may be connected.
it("offers to continue without a mailbox before any consent round trip", () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
  renderConnectAct();
  expect(
    screen.getByRole("button", { name: "Continue without a mailbox" }),
  ).toBeTruthy();
});

// Continue always presses. Without a mailbox it names the gap beside itself
// and goes nowhere: the step is not left by accident, and the reader is told
// what the honest exit beside it is for.
it("names the missing mailbox when Continue is pressed without one", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
  const { dispatch, persist } = renderConnectAct();
  await userEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    /A mailbox is still needed/,
  );
  expect(dispatch).not.toHaveBeenCalled();
  expect(persist).not.toHaveBeenCalled();
});

// After a successful consent it is no longer true, and recording the step as
// skipped would persist a fact contradicted by the roster.
it("stops offering the mailbox-less exit once consent has returned", async () => {
  stubWithSession({
    "GET /connectors": rosterWith({ state: "done" }),
  });
  renderConnectAct("ok");
  await screen.findByText("Live and capturing");
  expect(
    screen.queryByRole("button", { name: "Continue without a mailbox" }),
  ).toBeNull();
});

// A returning "ok" whose provider the roster never confirms is NOT a
// completed connection — the panel's own "Continue" fallback would otherwise
// be the only way out of a mailbox that is not actually connected. The honest
// exit (recorded truthfully as no mailbox) has to stay reachable until a live
// mailbox is confirmed.
it("keeps the mailbox-less exit open when consent returned but no mailbox could be confirmed", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
  renderConnectAct("ok");
  await screen.findByText("We couldn't confirm the connection.");
  expect(
    screen.getByRole("button", { name: "Continue without a mailbox" }),
  ).toBeInTheDocument();
});

// LinkedIn lives beside mail on the same screen: saving or skipping it never
// touches the mail flow above, and neither dispatch is CONNECT_DONE or a
// wizard-state write — the mail gate owns those exclusively. It is not a
// connection: the member writes down which profile their imported network is
// attributed to, and nothing is authorized.
describe("the LinkedIn card", () => {
  const saves: unknown[] = [];
  beforeEach(() => {
    saves.length = 0;
    stubWithSession({
      "GET /connectors": () => jsonResponse({ data: [] }),
      "PUT /me/linkedin-account": (body: unknown) => {
        saves.push(body);
        return jsonResponse({ connected: false, connections: 0 });
      },
    });
  });

  it("keeps the profile form closed until its card is clicked", () => {
    renderConnectAct();
    expect(screen.getByText("save →")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save profile" })).toBeNull();
  });

  it("saves the profile the network is attributed to, and authorizes nothing", async () => {
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));

    // The ask opens as its own dialog, same as a mail provider's, and says
    // where the connections themselves come from.
    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText(en["ob.conv.linkedin.importLater"])).toBeTruthy();
    expect(screen.queryByRole("button", { name: /authori[sz]e/i })).toBeNull();
    const button = screen.getByRole("button", { name: "Save profile" });
    expect(button).toBeDisabled();

    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    expect(button).not.toBeDisabled();
    await userEvent.click(button);

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
    // `connected: false` on the wire: a saved address grants nothing, and the
    // store keeps whatever authorization already exists.
    expect(saves).toEqual([
      { profile_url: "https://www.linkedin.com/in/lars", connected: false },
    ]);
  });

  it("can be declined from the open dialog, without a profile", async () => {
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.click(
      screen.getByRole("button", { name: "Skip LinkedIn for now" }),
    );
    expect(dispatch).toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });
  });

  it("shows the resolved state and offers no further action once skipped", () => {
    renderConnectAct(undefined, "skipped");
    expect(screen.getByText("Skipped: add it later in Settings")).toBeTruthy();
    expect(screen.getByRole("button", { name: /LinkedIn/ })).toBeDisabled();
  });

  it("shows the resolved state and offers no further action once connected", () => {
    renderConnectAct(undefined, "saved");
    expect(screen.getByText("Profile saved")).toBeTruthy();
    // A saved address is not a connected integration, and the tile's own
    // affordance must not call it one.
    expect(screen.getByText("saved")).toBeTruthy();
    expect(screen.queryByText("connected")).toBeNull();
    expect(screen.getByRole("button", { name: /LinkedIn/ })).toBeDisabled();
  });

  // A failed save must stay visible and retryable, not vanish behind a
  // dialog that already closed on the click that failed.
  it("keeps the dialog open and shows the failure when the save fails", async () => {
    stubWithSession({
      "GET /connectors": () => jsonResponse({ data: [] }),
      "PUT /me/linkedin-account": () =>
        jsonResponse({ detail: "LinkedIn refused the profile." }, 422),
    });
    renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));

    expect(
      await screen.findByText("LinkedIn refused the profile."),
    ).toBeInTheDocument();
    // Still open and retryable — not silently dismissed on the failed click.
    expect(
      screen.getByRole("button", { name: "Save profile" }),
    ).toBeInTheDocument();
  });
});

// The way on is read and acted on in the same place: the work surface the
// reader has been looking at, not a chip surfaced beside the transcript.
describe("the way onward", () => {
  it("renders on the stage's rail — never as a thread chip", () => {
    stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
    renderConnectAct();
    const onward = screen.getByRole("button", { name: "Continue" });
    expect(onward.closest(".ob-stage-acts")).toBeTruthy();
    expect(onward.closest(".mw-thread")).toBeNull();
  });

  // With a mailbox live the step is left as connected: the skip flag is
  // false, and the act hands on to the preferences act rather than entering.
  it("records the step as connected and hands on when a mailbox is live", async () => {
    stubWithSession({ "GET /connectors": rosterWith({ state: "none" }) });
    const { dispatch, persist } = renderConnectAct();
    const onward = screen.getByRole("button", { name: "Continue" });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Continue without a mailbox" }),
      ).toBeNull(),
    );
    await userEvent.click(onward);
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "connect", connectSkipped: false }),
    );
  });
});

// The four step-level consent guarantees used to be a two-column table
// squeezed into the rail's ~250px column, wrapping into broken-looking text.
// They render on the artifact surface now, where the reader passes through them
// before authorizing anything — behind a fold that names them, like every other
// disclosure in the product, and reachable in one click from the scene rather
// than from inside a provider's dialog.
describe("the consent guarantees", () => {
  it("sit on the surface behind a named fold, not in the rail", async () => {
    stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
    renderConnectAct();

    const toggle = screen.getByText(en["ob.conv.connect.guaranteesToggle"]);
    const fold = toggle.closest("details");
    expect(fold?.open).toBe(false);
    await userEvent.click(toggle);
    expect(fold?.open).toBe(true);

    for (const key of [
      "ob.s4.scope1Lead",
      "ob.s4.scope2Lead",
      "ob.s4.scope3Lead",
      "ob.s4.scope4Lead",
    ] as const) {
      const found = screen.getByText(en[key]);
      expect(found.closest("details")).toBe(fold);
      expect(found.closest(".mw-thread")).toBeNull();
    }
  });
});

describe("the IMAP dialog", () => {
  it("carries only the real contract's fields — no invented SMTP host, port or TLS toggle", async () => {
    stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
    renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Any other mailbox/ }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(screen.getByLabelText("IMAP host")).toBeTruthy();
    expect(screen.getByLabelText("Port")).toBeTruthy();
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("App password")).toBeTruthy();
    expect(screen.getByLabelText("Mailbox")).toBeTruthy();
    // The prototype this dialog adapts shows SMTP host/port and a "Require
    // TLS" checkbox; the real connector contract has neither, so this form
    // does not invent widgets that would submit nothing — it carries no
    // checkbox at all.
    //
    // Scoped to the DIALOG, which is what the claim is about. The scene behind
    // it carries the overnight question's own checkbox, and a document-wide
    // count would read that as this form having grown a widget.
    expect(within(dialog).queryByLabelText(/smtp/i)).toBeNull();
    expect(within(dialog).queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("closes on 'Not now' without touching the required-step skip", async () => {
    stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
    const { dispatch, persist } = renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Any other mailbox/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    // Backing out of ONE provider's ask is not the same decision as skipping
    // the whole required mailbox step — neither fires here.
    expect(dispatch).not.toHaveBeenCalled();
    expect(persist).not.toHaveBeenCalled();
    // The card grid is still there, ready for a different pick.
    expect(screen.getByRole("button", { name: /Microsoft/ })).toBeTruthy();
  });
});

// The disabled "Not now"/"Skip" buttons only guard their own click. The
// dialog's other dismissal routes — X, Escape, backdrop — all resolve to the
// SAME close handler (`ConnectDialog` → `Modal`), so a success landing after
// one of those already backed the reader out would leave a connection stored
// against a decision the panel already promised. Each provider serializes
// its own decision by keeping that one handler from acting while its own
// save is in flight.
describe("dismissal during an in-flight connect request", () => {
  it("keeps the OAuth dialog open against Escape while its connect POST is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession({
      "GET /connectors": () => jsonResponse({ data: [] }),
      "POST /connectors/graph/connect": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /Microsoft/ }));
    await userEvent.click(
      screen.getByRole("button", {
        name: "Allow access to my Microsoft",
      }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();

    deferred.resolve?.(jsonResponse({}));
  });

  it("keeps the IMAP dialog open against Escape while its connect POST is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession({
      "GET /connectors": () => jsonResponse({ data: [] }),
      "POST /connectors/imap/connect": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    renderConnectAct();
    await userEvent.click(
      screen.getByRole("button", { name: /Any other mailbox/ }),
    );
    await userEvent.type(screen.getByLabelText("Email"), "me@example.com");
    await userEvent.type(screen.getByLabelText("App password"), "secret");
    await userEvent.click(
      screen.getByRole("button", { name: "Test and connect" }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();

    deferred.resolve?.(
      jsonResponse({
        connection: {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
        },
      }),
    );
    await screen.findByText(/mailbox connected/i);
  });

  // LinkedIn's skip and its save race the same way mail's dismiss does: a
  // successful PUT landing after a skip already dispatched would leave the
  // profile saved on the server against a machine state that says skipped,
  // with no way to tell the later LINKEDIN_SAVED dispatch apart from a stale
  // one.
  it("keeps the LinkedIn dialog open and Skip disabled while its save is pending", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stubWithSession({
      "GET /connectors": () => jsonResponse({ data: [] }),
      "PUT /me/linkedin-account": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Skip LinkedIn for now" }),
      ).toBeDisabled(),
    );

    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(dispatch).not.toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });

    deferred.resolve?.(jsonResponse({ connected: false, connections: 0 }));
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
  });
});

// A roster invalidation (IMAP's own successful connect fires one) puts the
// query back into flight exactly like the first load did — the "pick one"
// rule the cards enforce cannot tell that refetch apart from a first read
// still pending, so both must withhold every card, not only the first.
it("keeps mail provider cards disabled during a roster refetch, not just its first load", async () => {
  const deferred: { resolve: ((r: Response) => void) | null } = {
    resolve: null,
  };
  let rosterCalls = 0;
  stubWithSession({
    "GET /connectors": () => {
      rosterCalls += 1;
      if (rosterCalls === 1) {
        return jsonResponse({ data: [] });
      }
      return new Promise((resolve) => {
        deferred.resolve = resolve;
      });
    },
    "POST /connectors/imap/connect": () =>
      jsonResponse({
        connection: {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
        },
      }),
  });
  renderConnectAct();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );

  await userEvent.click(
    screen.getByRole("button", { name: /Any other mailbox/ }),
  );
  await userEvent.type(screen.getByLabelText("Email"), "me@example.com");
  await userEvent.type(screen.getByLabelText("App password"), "secret");
  await userEvent.click(
    screen.getByRole("button", { name: "Test and connect" }),
  );
  await screen.findByText(/mailbox connected/i);

  // The connect just invalidated the shared roster query; its refetch (call
  // #2, deferred above) is in flight, so Google/Microsoft — still, on the
  // stale data, unconnected — must not be openable until it actually reports
  // back.
  expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled();
  expect(screen.getByRole("button", { name: /Microsoft/ })).toBeDisabled();

  deferred.resolve?.(
    jsonResponse({
      data: [
        {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
          backfill: { state: "none" },
        },
      ],
    }),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled(),
  );
});

// A failed roster read is the ONE mail-gate failure with no other surface to
// explain it: every card renders disabled either way, so a reader facing
// them with no failed read has nothing to act on and no way to tell it apart
// from an ordinary still-loading moment.
it("says why every mail card is disabled when the roster read fails, and offers a retry", async () => {
  let rosterCalls = 0;
  stubWithSession({
    "GET /connectors": () => {
      rosterCalls += 1;
      if (rosterCalls === 1) {
        return jsonResponse({ code: "internal" }, 500);
      }
      return jsonResponse({ data: [] });
    },
  });
  renderConnectAct();

  expect(
    await screen.findByText("Could not check your mailboxes"),
  ).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled();

  await userEvent.click(screen.getByRole("button", { name: "Retry" }));

  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );
  expect(
    screen.queryByText("Could not check your mailboxes"),
  ).not.toBeInTheDocument();
});

// The overnight question rides with this step: it is preselected, and the
// answer is written when the step COMPLETES rather than when the box is
// ticked. Both halves matter and neither is visible from the component that
// draws the checkbox, which owns no state of its own.

it("asks the overnight question preselected, beside the mailboxes", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) });
  renderConnectAct();
  const box = await screen.findByTestId("overnight-grant-choice");
  // Preselected: the features it feeds are the ones the product opens on, so
  // an unticked default ships an installation whose morning brief is
  // permanently empty for reasons nobody is told.
  expect(box).toHaveProperty("checked", true);
  // And nothing is granted yet — ticking states an intent this step carries.
  expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
});

it("grants nothing when the reader skips connecting a mailbox", async () => {
  const grants: unknown[] = [];
  stubWithSession({
    "GET /connectors": () => jsonResponse({ data: [] }),
    "PUT /me/agent-grants/morning_brief": (body: unknown) => {
      grants.push(body);
      return jsonResponse({});
    },
  });
  const { dispatch } = renderConnectAct();

  const without = screen.getByRole("button", {
    name: "Continue without a mailbox",
  });
  await waitFor(() => expect(without).not.toBeDisabled());
  await userEvent.click(without);
  // The agent reads their mail to build the brief, so authority over a mailbox
  // that was never connected is authority over nothing — recorded as though
  // the rep had agreed to something real. The box stays ticked; going on
  // without a mailbox is what decides.
  await waitFor(() =>
    expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
  );
  expect(grants).toEqual([]);
});

it("keeps an opt-out across the OAuth round trip", async () => {
  const answers: unknown[] = [];
  stubWithSession({
    "GET /connectors": () => jsonResponse({ data: [] }),
    "PUT /me/agent-grants/morning_brief": (body: unknown) => {
      answers.push(body);
      return jsonResponse({});
    },
  });

  // The rep unticks the box, then leaves for the provider's consent screen.
  const first = renderConnectAct();
  await userEvent.click(screen.getByTestId("overnight-grant-choice"));
  first.unmount();

  // A real "allow" leaves the page entirely, so the component remounts on the
  // way back. Without the remembered answer it would come back at its
  // preselected default and grant against an explicit opt-out.
  renderConnectAct();
  const box = await screen.findByTestId("overnight-grant-choice");
  expect(box).toHaveProperty("checked", false);
});

it("records a decline, rather than leaving it unanswered", async () => {
  const answers: unknown[] = [];
  stubWithSession({
    "GET /connectors": rosterWith({ state: "none" }),
    "PUT /me/agent-grants/morning_brief": (body: unknown) => {
      answers.push(body);
      return jsonResponse({});
    },
  });
  renderConnectAct(undefined, "skipped");

  await userEvent.click(screen.getByTestId("overnight-grant-choice"));
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Continue without a mailbox" }),
    ).toBeNull(),
  );
  await userEvent.click(screen.getByRole("button", { name: "Continue" }));

  // Declined and never-asked are different states, and the product asks once:
  // leaving an opt-out unanswered is what makes it ask again every night.
  await waitFor(() => expect(answers).toEqual([{ granted: false }]));
});
