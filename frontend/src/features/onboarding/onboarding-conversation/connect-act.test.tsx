/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { renderConnectAct, rosterWith } from "./connect-act.testkit";

// The Microsoft chip opens the same live OAuth panel as Google. The
// post-consent return view reads the roster fresh, whichever chip was open
// when the redirect left.
//
// The act owns the finish, and only the surface's own Continue reaches it.
// The backread after a confirmed connection can run on or be declined. Either
// way its dialog closes back onto the cards (LinkedIn still waiting), rather
// than carrying the reader out of the step. The step completes once, from the
// surface. CONNECT_DONE is the only event the mail path dispatches, and a
// second one would move the machine out from under the wizard.

// Every stub routes the session probe, because ConnectAct mounts
// capability-aware chrome and story-utils refuses to guess a session. An
// unrouted GET /me answers 501, every grant fails closed, and the provider
// cards stay disabled for a reason no assertion is about. The grants are
// empty because this act is onboarding, before any of them are held.

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
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
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
      name: "Connect Microsoft",
    }),
  ).toBeTruthy();
});

// The roster says which providers this installation can actually start a
// connect for, decided by the same predicate the connect endpoint uses. A card
// that opened anyway would send the reader through a dialog to a 501 they can
// do nothing about, so an unconnectable provider stops being a button and says
// what is missing instead.
it("does not offer a provider whose OAuth app is not registered", async () => {
  stubWithSession(
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [],
          providers: [
            { provider: "gmail", reason: "ready" },
            { provider: "graph", reason: "app_missing" },
            { provider: "imap", reason: "ready" },
          ],
        }),
    },
    {},
  );
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
  stubWithSession(
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [],
          providers: [{ provider: "graph", reason: "app_unusable" }],
        }),
    },
    {},
  );
  renderConnectAct();

  expect(
    await screen.findByText(/Microsoft app cannot be opened/),
  ).toBeTruthy();
  expect(screen.queryByText(/has not registered/)).toBeNull();
});

// Nothing in Settings fixes a deployment that does not serve the provider at
// all, so the card offers no link to a form that would have nothing in it.
it("offers no settings link for a provider this deployment does not serve", async () => {
  stubWithSession(
    {
      "GET /connectors": () =>
        jsonResponse({
          data: [],
          providers: [{ provider: "graph", reason: "unsupported" }],
        }),
    },
    {},
  );
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
  stubWithSession(
    {
      "GET /connectors": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    },
    {},
  );
  renderConnectAct();

  for (const name of [/Google/, /Microsoft/, /Other mailbox/]) {
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
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ code: "internal" }, 500),
    },
    {},
  );
  renderConnectAct();

  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled(),
  );
  expect(screen.getByRole("button", { name: /Microsoft/ })).toBeDisabled();
  expect(screen.getByRole("button", { name: /Other mailbox/ })).toBeDisabled();
});

// The mark is what tells a genuine return apart from a stale or bookmarked
// outcome URL (see `attemptedProvider` in connect-act.tsx) — it has to be
// written before the redirect actually leaves, not after.
it("marks this tab's own attempt before the real redirect fires", async () => {
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ data: [] }),
      "POST /connectors/graph/connect": () =>
        jsonResponse({ authorize_url: "https://login.microsoftonline/x" }),
    },
    {},
  );
  const assign = vi.fn();
  vi.stubGlobal("location", { ...globalThis.location, assign });
  renderConnectAct();
  await userEvent.click(screen.getByRole("button", { name: /Microsoft/ }));
  await userEvent.click(
    screen.getByRole("button", { name: "Connect Microsoft" }),
  );
  await waitFor(() => expect(assign).toHaveBeenCalled());
  expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBe("graph");
});

it("renders the provider-agnostic return view on OAuth return", async () => {
  stubWithSession(
    {
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
    },
    {},
  );
  renderConnectAct("ok");
  expect(await screen.findByText("Live and capturing")).toBeTruthy();
});

it("asks how far back to read once the mailbox is confirmed", async () => {
  stubWithSession(
    {
      "GET /connectors": rosterWith({ state: "none" }),
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          estimated_messages: 900,
          computed_at: "2026-07-31T10:00:00Z",
        }),
    },
    {},
  );
  renderConnectAct("ok");
  expect(
    await screen.findByRole("heading", {
      name: "Import period",
    }),
  ).toBeTruthy();
  // Exactly one history-read decision on the surface — the standalone
  // Settings-style backfill panel is not a second one stacked beside it.
  expect(screen.queryByText("Import mailbox history")).toBeNull();
  expect(
    screen.getAllByRole("heading", { name: "Import period" }),
  ).toHaveLength(1);
});

// Leaving the backread with the read still running puts the reader back on
// the surface, LinkedIn card and all: the step is not left until they press
// its own Continue, which then records the mailbox as connected.
it("closes the backread onto the surface and finishes only from the surface's Continue", async () => {
  stubWithSession(
    {
      "GET /connectors": rosterWith({
        state: "running",
        counts: { messages_scanned: 12 },
      }),
    },
    {},
  );
  const { dispatch, persist } = renderConnectAct("ok");

  await userEvent.click(
    await screen.findByRole("button", { name: "Continue during import" }),
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
  // The mailbox IS connected on this path, so the connect step was not
  // skipped — and leaving the last stop is what records completion.
  expect(persist).toHaveBeenCalledWith(
    expect.objectContaining({ step: "complete", connectSkipped: false }),
  );
});

it("declining the history read closes the result without starting one or leaving the step", async () => {
  const starts: unknown[] = [];
  stubWithSession(
    {
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
    },
    {},
  );
  const { dispatch } = renderConnectAct("ok");

  await userEvent.click(
    await screen.findByRole("button", { name: "Skip history import" }),
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
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
  renderConnectAct();
  expect(
    screen.getByRole("button", { name: "Continue without a mailbox" }),
  ).toBeTruthy();
});

// Continue always presses. Without a mailbox it names the gap beside itself
// and goes nowhere: the step is not left by accident, and the reader is told
// what the honest exit beside it is for.
it("names the missing mailbox when Continue is pressed without one", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
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
  stubWithSession(
    {
      "GET /connectors": rosterWith({ state: "done" }),
    },
    {},
  );
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
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
  renderConnectAct("ok");
  await screen.findByText("The connection could not be confirmed.");
  expect(
    screen.getByRole("button", { name: "Continue without a mailbox" }),
  ).toBeInTheDocument();
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
  stubWithSession(
    {
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
    },
    {},
  );
  renderConnectAct();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );

  await userEvent.click(screen.getByRole("button", { name: /Other mailbox/ }));
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
  stubWithSession(
    {
      "GET /connectors": () => {
        rosterCalls += 1;
        if (rosterCalls === 1) {
          return jsonResponse({ code: "internal" }, 500);
        }
        return jsonResponse({ data: [] });
      },
    },
    {},
  );
  renderConnectAct();

  expect(
    await screen.findByText("Mailboxes could not be checked"),
  ).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /Google/ })).toBeDisabled();

  await userEvent.click(screen.getByRole("button", { name: "Retry" }));

  await waitFor(() =>
    expect(screen.getByRole("button", { name: /Google/ })).not.toBeDisabled(),
  );
  expect(
    screen.queryByText("Mailboxes could not be checked"),
  ).not.toBeInTheDocument();
});

// The overnight question rides with this step: it is preselected, and the
// answer is written when the step COMPLETES rather than when the box is
// ticked. Both halves matter and neither is visible from the component that
// draws the checkbox, which owns no state of its own.

it("asks the overnight question preselected, beside the mailboxes", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
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
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ data: [] }),
      "PUT /me/agent-grants/morning_brief": (body: unknown) => {
        grants.push(body);
        return jsonResponse({});
      },
    },
    {},
  );
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
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ data: [] }),
      "PUT /me/agent-grants/morning_brief": (body: unknown) => {
        answers.push(body);
        return jsonResponse({});
      },
    },
    {},
  );

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
  stubWithSession(
    {
      "GET /connectors": rosterWith({ state: "none" }),
      "PUT /me/agent-grants/morning_brief": (body: unknown) => {
        answers.push(body);
        return jsonResponse({});
      },
    },
    {},
  );
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

describe("returning to the dialog a proven attempt left from", () => {
  it("reopens the same dialog, showing the result rather than a fresh ask", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "graph");
    stubWithSession(
      {
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
      },
      {},
    );
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toBeTruthy();
    // The plain provider name, not the pre-consent "access needed" ask:
    // nothing is being requested at this point.
    expect(
      screen.getByRole("heading", { name: "Microsoft" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    // Consumed on read: a reload of this same URL would find no mark.
    expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBeNull();
  });

  it("falls back to the plain inline result when the URL's provider does not match this tab's own attempt", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "gmail");
    stubWithSession(
      {
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
      },
      {},
    );
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("falls back to the plain inline result when no mark is recorded at all — a stale or bookmarked link", async () => {
    stubWithSession(
      {
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
      },
      {},
    );
    renderConnectAct("ok", "pending", "cn.consent", "graph");

    expect(await screen.findByText("Live and capturing")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
