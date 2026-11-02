/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import {
  jsonResponse,
  type RouteMap,
  stubWithSession,
} from "../../../testing/story-utils";
import { ConnectAct } from "./connect-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The connect step once a mailbox is live, and the extras beside it in either
// state. The page carries one primary in every state. The import press starts
// the run and leaves, declining it leaves with nothing started, and a run
// already going leaves plain Continue. The posture and a calendar redirect in
// flight hold the way on, because leaving mid-write lands under either answer.

function renderConnectAct(
  linkedinStatus: ConversationState["linkedinStatus"] = "pending",
  landing: { outcome?: string; provider?: string } = {},
) {
  const state: ConversationState = {
    ...initialConversationState,
    act: "connect",
    phase: "cn.consent",
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
          outcome={landing.outcome}
          returningProvider={landing.provider}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, dispatch, persist };
}

// A row carries the grant its service needs (sending for the mailbox, writing
// for the calendar) unless a test drops it.
const GRANTED: Readonly<Record<string, string[]>> = {
  gcal: ["https://www.googleapis.com/auth/calendar.events"],
  gmail: ["read", "https://www.googleapis.com/auth/gmail.send"],
};
const row = (provider: string, extra: Record<string, unknown> = {}) => ({
  id: provider,
  provider,
  status: "connected",
  scopes: GRANTED[provider] ?? ["read"],
  account_label: "lars@example.org",
  ...extra,
});

const roster =
  (...rows: Record<string, unknown>[]) =>
  () =>
    jsonResponse({ data: rows });

const preview = () =>
  jsonResponse({
    window: "6m",
    estimated_messages: 900,
    computed_at: "2026-07-31T10:00:00Z",
  });

function stub(routes: RouteMap) {
  stubWithSession(routes, {});
}

const IMPORT = "Import 6 months and continue";

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

describe("a live Google mailbox", () => {
  it("shows the account, its posture and the import, with one primary", async () => {
    stub({
      "GET /connectors": roster(
        row("gmail", { backfill: { state: "none" } }),
        row("gcal"),
      ),
      "POST /connectors/gmail/backfill/preview": preview,
    });
    const { container } = renderConnectAct();

    expect(await screen.findByText("Google is connected")).toBeInTheDocument();
    // One account row, the way Connections draws it, not a row per service.
    expect(screen.getAllByText("lars@example.org")).toHaveLength(1);
    expect(
      screen.getByText(en["ob.conv.connect.importTitle"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /Held until checked/ }),
    ).toBeChecked();
    await screen.findByRole("button", { name: IMPORT });
    expect(container.querySelectorAll(".btn-primary")).toHaveLength(1);
    // The ask is answered: no account picker and no skip beside a mailbox.
    expect(screen.queryByRole("radio", { name: /Google/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Skip for now" })).toBeNull();
  });

  // Setup asks whether the mailbox is connected at all. A sync the sweep keeps
  // retrying does not send the member back to the account chooser.
  it("keeps a mailbox whose sync is failing connected", async () => {
    stub({
      "GET /connectors": roster(
        row("gmail", { status: "error", backfill: { state: "none" } }),
        row("gcal"),
      ),
      "POST /connectors/gmail/backfill/preview": preview,
    });
    renderConnectAct();

    expect(await screen.findByText("Google is connected")).toBeInTheDocument();
    expect(screen.queryByRole("radio", { name: /Google/ })).toBeNull();
  });

  it("imports the chosen window, then records the step as connected", async () => {
    const starts: unknown[] = [];
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "none" } })),
      "POST /connectors/gmail/backfill/preview": preview,
      "POST /connectors/gmail/backfill": (body) => {
        starts.push(body);
        return jsonResponse({ state: "queued", window: "6m" }, 202);
      },
    });
    const { dispatch, persist } = renderConnectAct();

    await userEvent.click(await screen.findByRole("button", { name: IMPORT }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
    expect(starts).toEqual([{ window: "6m" }]);
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete", connectSkipped: false }),
    );
  });

  it("continues without importing and starts nothing", async () => {
    const starts: unknown[] = [];
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "none" } })),
      "POST /connectors/gmail/backfill/preview": preview,
      "POST /connectors/gmail/backfill": (body) => {
        starts.push(body);
        return jsonResponse({ state: "queued" }, 202);
      },
    });
    const { dispatch, persist } = renderConnectAct();

    await userEvent.click(
      await screen.findByRole("button", { name: "Continue without importing" }),
    );

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
    expect(starts).toEqual([]);
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ connectSkipped: false }),
    );
  });

  it("stays on the step when the import refuses to start", async () => {
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "none" } })),
      "POST /connectors/gmail/backfill/preview": preview,
      "POST /connectors/gmail/backfill": () =>
        jsonResponse({ code: "backfill_running", detail: "Already on." }, 409),
    });
    const { dispatch } = renderConnectAct();

    await userEvent.click(await screen.findByRole("button", { name: IMPORT }));

    expect(
      await screen.findByText(/The mailbox history import did not start/),
    ).toBeInTheDocument();
    expect(dispatch).not.toHaveBeenCalled();
  });

  // A reload after the import started lands here with the run going. Starting a
  // second one is not on offer, and Continue is the way on.
  it("leaves plain Continue over a run already going", async () => {
    stub({
      "GET /connectors": roster(
        row("gmail", {
          backfill: { state: "running", counts: { messages_scanned: 12 } },
        }),
      ),
    });
    const { container, dispatch } = renderConnectAct();

    await userEvent.click(
      await screen.findByRole("button", { name: "Continue" }),
    );

    expect(screen.queryByRole("button", { name: IMPORT })).toBeNull();
    expect(container.querySelectorAll(".btn-primary")).toHaveLength(1);
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
  });

  // The calendar leg alone: the mailbox is live and the import stays the
  // footer's choice, so the press carries neither a chain nor a window.
  it("asks for the calendar alone when the account has none", async () => {
    const bodies: unknown[] = [];
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "done" } })),
      "POST /connectors/gcal/connect": (body) => {
        bodies.push(body);
        return jsonResponse({});
      },
    });
    renderConnectAct();

    expect(
      await screen.findByText(en["ob.conv.connect.calendarNotConnected"]),
    ).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Connect calendar" }),
    );
    await waitFor(() => expect(bodies).toEqual([{ return_to: "onboarding" }]));
    expect(
      screen.getByText(en["ob.conv.connect.connectedCaptionMailOnly"]),
    ).toBeInTheDocument();
  });

  // A read-only grant cannot add bookings, so it asks for a reconnect the way
  // Connections and Meetings do, never a plain Connected.
  it("asks to reconnect a calendar connected for reading only", async () => {
    stub({
      "GET /connectors": roster(
        row("gmail", { backfill: { state: "done" } }),
        row("gcal", { scopes: ["read"] }),
      ),
    });
    renderConnectAct();

    expect(
      await screen.findByText(en["scheduling.readOnlyBadge"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(en["scheduling.readOnlyCalendar"]),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeEnabled();
    expect(
      screen.getByText(en["connectors.health.attention"]),
    ).toBeInTheDocument();
  });

  // A chained press whose calendar leg was refused lands on the calendar's
  // outcome with the mailbox already live. The page is the connected one, the
  // refusal is named, and the calendar can be asked for again on its own.
  it("keeps the mailbox and offers the calendar again after a refused calendar leg", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "gcal");
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "none" } })),
      "POST /connectors/gmail/backfill/preview": preview,
    });
    const { container } = renderConnectAct("pending", {
      outcome: "denied",
      provider: "gcal",
    });

    expect(await screen.findByText("Google is connected")).toBeInTheDocument();
    expect(
      await screen.findByText(en["ob.conv.connect.calendarFailedTitle"]),
    ).toBeInTheDocument();
    expect(screen.queryByText(en["ob.s4.connectDenied"])).toBeNull();
    expect(
      screen.getByText(en["ob.conv.connect.calendarNotConnected"]),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Connect calendar" }),
    ).toBeInTheDocument();
    await screen.findByRole("button", { name: IMPORT });
    expect(container.querySelectorAll(".btn-primary")).toHaveLength(1);
    expect(screen.queryByRole("radio", { name: /Google/ })).toBeNull();
  });

  it("holds the way on while the posture is being saved", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "none" } })),
      "POST /connectors/gmail/backfill/preview": preview,
      "PUT /connectors/gmail/mail-posture": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    renderConnectAct();

    await userEvent.click(
      await screen.findByRole("radio", { name: /Always held/ }),
    );

    await waitFor(() =>
      expect(screen.getByRole("button", { name: IMPORT })).toBeDisabled(),
    );
    expect(
      screen.getByRole("button", { name: "Continue without importing" }),
    ).toBeDisabled();
    deferred.resolve?.(jsonResponse({}));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: IMPORT })).not.toBeDisabled(),
    );
  });
});

// Only Google and Microsoft mailboxes can be read backward from a date.
it("offers no import for an IMAP mailbox", async () => {
  const previews: unknown[] = [];
  stub({
    "GET /connectors": roster(row("imap")),
    "POST /connectors/imap/backfill/preview": (body) => {
      previews.push(body);
      return jsonResponse({}, 422);
    },
  });
  renderConnectAct();

  expect(
    await screen.findByRole("button", { name: "Continue" }),
  ).toBeInTheDocument();
  expect(screen.queryByText(en["ob.conv.connect.importTitle"])).toBeNull();
  expect(screen.queryByRole("button", { name: "Connect calendar" })).toBeNull();
  expect(previews).toEqual([]);
});

// LinkedIn lives beside mail on the same screen: saving or skipping it never
// touches the mail flow, and nothing is authorized. It is a row whose ask
// opens in a dialog of its own.
describe("the LinkedIn row", () => {
  const saves: unknown[] = [];
  const saveRoute =
    (answer: () => Response | Promise<Response>) => (body: unknown) => {
      saves.push(body);
      return answer();
    };
  beforeEach(() => {
    saves.length = 0;
  });

  it("saves the profile the network is attributed to, and authorizes nothing", async () => {
    stub({
      "GET /connectors": roster(),
      "PUT /me/linkedin-account": saveRoute(() =>
        jsonResponse({ connected: false, connections: 0 }),
      ),
    });
    const { dispatch } = renderConnectAct();
    expect(screen.queryByRole("dialog")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Add profile" }));

    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText(en["ob.conv.linkedin.importLater"])).toBeTruthy();
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
    // `connected: false` on the wire: a saved address grants nothing.
    expect(saves).toEqual([
      { profile_url: "https://www.linkedin.com/in/lars", connected: false },
    ]);
  });

  it("can be declined from the open dialog, without a profile", async () => {
    stub({ "GET /connectors": roster() });
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: "Add profile" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Skip LinkedIn for now" }),
    );
    expect(dispatch).toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });
  });

  it("says what was settled and offers nothing more", () => {
    stub({ "GET /connectors": roster() });
    renderConnectAct("saved");
    expect(screen.getByText("Profile saved")).toBeTruthy();
    // A saved address is not a connected integration.
    expect(screen.getByText("Saved")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add profile" })).toBeNull();
    cleanup();
    renderConnectAct("skipped");
    expect(screen.getByText("Skipped: add it later in Settings")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add profile" })).toBeNull();
  });

  it("keeps the dialog open and shows the failure when the save fails", async () => {
    stub({
      "GET /connectors": roster(),
      "PUT /me/linkedin-account": () =>
        jsonResponse({ detail: "LinkedIn refused the profile." }, 422),
    });
    renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: "Add profile" }));
    await userEvent.type(
      screen.getByLabelText("Your LinkedIn profile URL"),
      "https://www.linkedin.com/in/lars",
    );
    await userEvent.click(screen.getByRole("button", { name: "Save profile" }));

    expect(
      await screen.findByText("LinkedIn refused the profile."),
    ).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  // The skip and the save answer one question: a PUT landing after a skip
  // already dispatched would leave two answers stored.
  it("holds every dismissal while the save is in flight", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stub({
      "GET /connectors": roster(),
      "PUT /me/linkedin-account": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const { dispatch } = renderConnectAct();
    await userEvent.click(screen.getByRole("button", { name: "Add profile" }));
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
    deferred.resolve?.(jsonResponse({ connected: false, connections: 0 }));
    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({
        type: "LINKEDIN_SAVED",
        profile: "https://www.linkedin.com/in/lars",
      }),
    );
    expect(dispatch).not.toHaveBeenCalledWith({ type: "LINKEDIN_SKIPPED" });
  });
});

// The overnight question rides with this step: preselected, and written when
// the step COMPLETES rather than when the switch flips.
describe("the overnight question", () => {
  it("is asked preselected, and grants nothing yet", async () => {
    stub({ "GET /connectors": roster() });
    renderConnectAct();
    expect(await screen.findByTestId("overnight-grant-choice")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.queryByText(en["overnightGrant.danger"])).toBeNull();
  });

  // A real "allow" leaves the page, so the act remounts on the way back.
  // Without the remembered answer it would return at its default and grant
  // against an explicit opt-out.
  it("keeps an opt-out across the OAuth round trip", async () => {
    stub({ "GET /connectors": roster() });
    const first = renderConnectAct();
    await userEvent.click(screen.getByTestId("overnight-grant-choice"));
    first.unmount();

    renderConnectAct();
    expect(await screen.findByTestId("overnight-grant-choice")).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  // Declined and never-asked are different states, and the product asks once.
  it("records a decline, rather than leaving it unanswered", async () => {
    const answers: unknown[] = [];
    stub({
      "GET /connectors": roster(row("gmail", { backfill: { state: "done" } })),
      "PUT /me/agent-grants/morning_brief": (body: unknown) => {
        answers.push(["morning_brief", body]);
        return jsonResponse({});
      },
      "PUT /me/agent-grants/overnight_at_risk_sweep": (body: unknown) => {
        answers.push(["overnight_at_risk_sweep", body]);
        return jsonResponse({});
      },
    });
    renderConnectAct("skipped");

    const onward = await screen.findByRole("button", { name: "Continue" });
    await userEvent.click(screen.getByTestId("overnight-grant-choice"));
    expect(screen.getByTestId("overnight-grant-choice")).toHaveAttribute(
      "aria-checked",
      "false",
    );
    await userEvent.click(onward);

    // One answer, written for both overnight agents.
    await waitFor(() =>
      expect(answers).toEqual([
        ["morning_brief", { granted: false }],
        ["overnight_at_risk_sweep", { granted: false }],
      ]),
    );
  });
});
