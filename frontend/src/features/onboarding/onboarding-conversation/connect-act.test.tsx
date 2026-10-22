/** @vitest-environment happy-dom */
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
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { ConnectAct } from "./connect-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The post-consent return view reads the roster fresh, whichever account was
// picked when the redirect left. The act also owns the finish, and only the
// surface's own way onward reaches it. CONNECT_DONE is the only event the mail
// path dispatches, and a second one would move the machine out from under the
// wizard. The ask is proven in connect-act.chain.test.tsx, the live mailbox and
// the extras beside it in connect-act.connected.test.tsx.

// Every stub in this file routes the session probe, because ConnectAct mounts
// capability-aware chrome and story-utils refuses to guess a session. An
// unrouted GET /me answers 501 and every grant fails closed. The provider
// cards then stay disabled for a reason no assertion here is about. The grants
// are empty because this act is onboarding, before any of them are held.

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

// Nothing in Settings fixes a deployment that does not serve the provider at
// all, so the account cannot be picked and offers nothing to press. The
// accounts an administrator can unblock are proven in
// connect-act.app-missing.test.tsx.
it("refuses a provider this deployment does not serve", async () => {
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
  expect(screen.getByRole("radio", { name: /Microsoft/ })).toBeDisabled();
  expect(screen.queryByRole("link")).toBeNull();
});

// An account left pickable while the roster is unread would let a reader
// connect a second mailbox before the fetch ever reports the first.
it("withholds every account until the roster load settles", async () => {
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

  for (const name of [/Google/, /Microsoft/, /Other mail provider/]) {
    expect(screen.getByRole("radio", { name })).toBeDisabled();
  }

  deferred.resolve?.(jsonResponse({ data: [] }));
  await waitFor(() =>
    expect(screen.getByRole("radio", { name: /Google/ })).not.toBeDisabled(),
  );
});

// A roster fetch that failed reports the same "nothing confirmed yet" fact as
// one still loading.
it("withholds every account when the roster fetch fails", async () => {
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ code: "internal" }, 500),
    },
    {},
  );
  renderConnectAct();

  await waitFor(() =>
    expect(screen.getByRole("radio", { name: /Google/ })).toBeDisabled(),
  );
  expect(screen.getByRole("radio", { name: /Microsoft/ })).toBeDisabled();
  expect(
    screen.getByRole("radio", { name: /Other mail provider/ }),
  ).toBeDisabled();
});

// The mark tells a real return apart from a stale or bookmarked outcome URL
// (see `returnedVendor` in connect-act.tsx). So it has to be written before
// the redirect leaves, not after.
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
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("radio", { name: /Microsoft/ })).not.toBeDisabled(),
  );
  await user.click(screen.getByRole("radio", { name: /Microsoft/ }));
  await user.click(
    screen.getByRole("button", { name: "Connect mail and calendar" }),
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
  expect(await screen.findByText(en["connectors.svc.newMail"])).toBeTruthy();
});

// The way past a missing mailbox is offered while connecting is still the
// open question.
it("offers to skip before any consent round trip", () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
  renderConnectAct();
  expect(screen.getByRole("button", { name: "Skip for now" })).toBeTruthy();
});

// After a successful consent it is no longer true, and recording the step as
// skipped would persist a fact contradicted by the roster.
it("stops offering the skip once consent has returned", async () => {
  stubWithSession(
    {
      "GET /connectors": rosterWith({ state: "done" }),
    },
    {},
  );
  renderConnectAct("ok");
  await screen.findByText(en["connectors.svc.newMail"]);
  expect(screen.queryByRole("button", { name: "Skip for now" })).toBeNull();
});

// A returning "ok" whose provider the roster never confirms is NOT a
// completed connection — the panel's own "Continue" fallback would otherwise
// be the only way out of a mailbox that is not actually connected. The honest
// exit (recorded truthfully as no mailbox) has to stay reachable until a live
// mailbox is confirmed.
it("keeps the skip open when consent returned but no mailbox could be confirmed", async () => {
  stubWithSession({ "GET /connectors": () => jsonResponse({ data: [] }) }, {});
  renderConnectAct("ok");
  await screen.findByText("The connection could not be confirmed.");
  expect(
    screen.getByRole("button", { name: "Skip for now" }),
  ).toBeInTheDocument();
});

// The way on is read and acted on in the same place. That is the work surface
// the reader has been looking at, not a chip beside the transcript.
describe("the way onward", () => {
  it("renders on the stage's rail — never as a thread chip", () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    renderConnectAct();
    const onward = screen.getByRole("button", { name: "Skip for now" });
    expect(onward.closest(".ob-stage-acts")).toBeTruthy();
  });
});

describe("the IMAP form", () => {
  it("carries only the real contract's fields — no invented SMTP host, port or TLS toggle", async () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    renderConnectAct();
    await userEvent.click(
      await screen.findByRole("radio", { name: /Other mail provider/ }),
    );

    const form = await screen.findByRole("region", {
      name: "Connect mail host",
    });
    expect(screen.getByLabelText("IMAP host")).toBeTruthy();
    expect(screen.getByLabelText("Port")).toBeTruthy();
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("App password")).toBeTruthy();
    expect(screen.getByLabelText("Mailbox")).toBeTruthy();
    // The real connector contract has no SMTP host or TLS toggle, so this
    // form invents no widget that would submit nothing. Scoped to the form:
    // the step around it carries the overnight question's own checkbox.
    expect(within(form).queryByLabelText(/smtp/i)).toBeNull();
    expect(within(form).queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("puts the form away on 'Not now' without skipping the step", async () => {
    stubWithSession(
      { "GET /connectors": () => jsonResponse({ data: [] }) },
      {},
    );
    const { dispatch, persist } = renderConnectAct();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("radio", { name: /Other mail provider/ }),
    );
    await user.click(screen.getByRole("button", { name: "Not now" }));

    expect(screen.queryByLabelText("IMAP host")).toBeNull();
    // Backing out of one provider's ask is not the same decision as skipping
    // the whole mailbox step: neither fires here.
    expect(dispatch).not.toHaveBeenCalled();
    expect(persist).not.toHaveBeenCalled();
    expect(screen.getByRole("radio", { name: /Microsoft/ })).not.toBeChecked();
  });
});

// The disabled Skip only guards its own click. The dialog's other dismissal
// routes (X, Escape, backdrop) resolve to the same close handler, so that
// handler refuses while the save is in flight.
// A roster invalidation (IMAP's own successful connect fires one) puts the
// query back into flight as the first load did. The "pick one" rule cannot
// tell that refetch apart from a first read still pending.
it("keeps the accounts disabled during a roster refetch, not just its first load", async () => {
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
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("radio", { name: /Google/ })).not.toBeDisabled(),
  );

  await user.click(screen.getByRole("radio", { name: /Other mail provider/ }));
  await user.type(screen.getByLabelText("Email"), "me@example.com");
  await user.type(screen.getByLabelText("App password"), "secret");
  await user.click(screen.getByRole("button", { name: "Test and connect" }));
  await waitFor(() => expect(rosterCalls).toBe(2));

  // The connect just invalidated the shared roster query, and its refetch (call
  // #2, deferred above) is in flight. Google and Microsoft, unconnected on the
  // stale data, must not be pickable until it reports back.
  expect(screen.getByRole("radio", { name: /Google/ })).toBeDisabled();
  expect(screen.getByRole("radio", { name: /Microsoft/ })).toBeDisabled();

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
  // The re-read roster is what closes the form, onto the live mailbox.
  expect(
    await screen.findByText(en["connectors.svc.newMail"]),
  ).toBeInTheDocument();
  expect(screen.queryByRole("radio", { name: /Google/ })).toBeNull();
});

// A failed roster read is the one mail-gate failure with no other surface to
// explain it. Every account renders disabled either way, which without a word
// reads as an ordinary still-loading moment.
it("says why every account is disabled when the roster read fails, and offers a retry", async () => {
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
  expect(screen.getByRole("radio", { name: /Google/ })).toBeDisabled();

  await userEvent.click(screen.getByRole("button", { name: "Retry" }));

  await waitFor(() =>
    expect(screen.getByRole("radio", { name: /Google/ })).not.toBeDisabled(),
  );
  expect(
    screen.queryByText("Mailboxes could not be checked"),
  ).not.toBeInTheDocument();
});

// The overnight question rides with this step: it is preselected, and the
// answer is written when the step COMPLETES rather than when the box is
// ticked. Both halves matter and neither is visible from the component that
// draws the checkbox, which owns no state of its own.

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

  const without = screen.getByRole("button", { name: "Skip for now" });
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
