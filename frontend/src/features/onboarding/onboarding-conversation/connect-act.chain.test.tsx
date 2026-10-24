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
import type { RouteMap } from "../../../testing/story-utils";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { ConnectAct } from "./connect-act";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// The connect step's ask: the work account picked once, mail and calendar
// asked for in one press. The calendar rides as the mail consent's
// `also_connect`, so the server chains the second consent onto the first's
// return. Onboarding never sends an import window, because it asks how far
// back to read only once the mailbox is live.

function renderAct(outcome?: string, returningProvider?: string) {
  const state: ConversationState = {
    ...initialConversationState,
    act: "connect",
    phase: "cn.consent",
  };
  const dispatch = vi.fn();
  const persist = vi.fn(async () => true);
  render(
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
  return { dispatch, persist };
}

/** A roster with nothing connected, plus whatever the test routes. */
function stub(routes: RouteMap = {}) {
  stubWithSession(
    { "GET /connectors": () => jsonResponse({ data: [] }), ...routes },
    {},
  );
}

/** Records every connect body per provider and answers with a consent URL. */
function recordConnects() {
  const bodies: { provider: string; body: unknown }[] = [];
  const routes: RouteMap = {};
  for (const provider of ["gmail", "gcal", "graph", "graphcal"]) {
    routes[`POST /connectors/${provider}/connect`] = (body: unknown) => {
      bodies.push({ provider, body });
      return jsonResponse({
        authorize_url: `https://consent.test/${provider}`,
      });
    };
  }
  return { bodies, routes };
}

async function pick(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  const radio = await screen.findByRole("radio", { name });
  await waitFor(() => expect(radio).not.toBeDisabled());
  await user.click(radio);
}

let assign: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
  assign = vi.fn();
  vi.stubGlobal("location", { ...globalThis.location, assign });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

describe("one press for mail and calendar", () => {
  it("asks for Google mail with its calendar chained, and no import window", async () => {
    const { bodies, routes } = recordConnects();
    stub(routes);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    await user.click(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    );

    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://consent.test/gmail"),
    );
    expect(bodies).toEqual([
      {
        provider: "gmail",
        body: { return_to: "onboarding", also_connect: "gcal" },
      },
    ]);
    // The mark names the mailbox: the trip lands on the calendar, which is
    // the same account.
    expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBe("gmail");
  });

  it("pairs Microsoft mail with its own calendar", async () => {
    const { bodies, routes } = recordConnects();
    stub(routes);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Microsoft/);

    await user.click(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    );

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({
      provider: "graph",
      body: { return_to: "onboarding", also_connect: "graphcal" },
    });
  });

  it("asks for mail alone, with no second consent, once Calendar is unticked", async () => {
    const { bodies, routes } = recordConnects();
    stub(routes);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);
    await user.click(screen.getByRole("checkbox", { name: /Calendar/ }));

    expect(
      screen.getByText("Google asks once, then brings you back here"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Connect mail" }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({
      provider: "gmail",
      body: { return_to: "onboarding" },
    });
  });

  it("asks for the calendar on its own consent when Mail is unticked", async () => {
    const { bodies, routes } = recordConnects();
    stub(routes);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);
    await user.click(screen.getByRole("checkbox", { name: /^Mail/ }));
    await user.click(screen.getByRole("button", { name: "Connect calendar" }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]).toEqual({
      provider: "gcal",
      body: { return_to: "onboarding" },
    });
  });

  it("refuses the press, and says why, with nothing ticked", async () => {
    const { bodies, routes } = recordConnects();
    stub(routes);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);
    await user.click(screen.getByRole("checkbox", { name: /^Mail/ }));
    await user.click(screen.getByRole("checkbox", { name: /Calendar/ }));

    expect(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    ).toBeDisabled();
    expect(screen.getByText(en["ob.conv.connect.pickService"])).toBeVisible();
    expect(bodies).toEqual([]);
  });
});

describe("the ask before the vendor's screens", () => {
  it("says the vendor asks twice when both are ticked", async () => {
    stub();
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Microsoft/);
    expect(
      screen.getByText("Microsoft asks twice, then brings you back here"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(en["ob.s4.microsoftUnverified"], { exact: false }),
    ).toBeInTheDocument();
  });

  it("keeps what connecting means behind a named fold", async () => {
    stub();
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    const toggle = screen.getByText(en["ob.conv.connect.meansTitle"]);
    const fold = toggle.closest("details");
    expect(fold?.open).toBe(false);
    for (const key of [
      "ob.s4.scope1Lead",
      "ob.s4.scope2Lead",
      "ob.s4.scope3Lead",
      "ob.s4.scope4Lead",
      "captureNotice.title",
    ] as const) {
      expect(screen.getByText(en[key]).closest("details")).toBe(fold);
    }
  });

  it("says so in place when this installation never wired the vendor", async () => {
    stub({
      "POST /connectors/gmail/connect": () =>
        jsonResponse({ code: "not_implemented" }, 501),
    });
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);
    await user.click(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    );

    expect(
      await screen.findByText("Google is not configured on this installation."),
    ).toBeInTheDocument();
    expect(assign).not.toHaveBeenCalled();
  });

  it("holds every other way off the ask while its connect is in flight", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    stub({
      "POST /connectors/gmail/connect": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const { persist } = renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);
    await user.click(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    );

    // A skip or another account landing while the consent URL is still on
    // its way would contradict the redirect that follows it.
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Skip for now" }),
      ).toBeDisabled(),
    );
    expect(screen.getByRole("radio", { name: /Microsoft/ })).toBeDisabled();
    expect(persist).not.toHaveBeenCalled();

    deferred.resolve?.(
      jsonResponse({ authorize_url: "https://consent.test/x" }),
    );
    await waitFor(() => expect(assign).toHaveBeenCalled());
  });
});

describe("the way onward before a mailbox", () => {
  // The ask's own Connect is the page's one primary; the rail offers only the
  // skip, and says the checklist will remind.
  it("puts no second primary on the rail", async () => {
    stub();
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    const primaries = document.querySelectorAll(".btn-primary");
    expect(primaries).toHaveLength(1);
    expect(primaries[0]).toHaveTextContent("Connect mail and calendar");
    const rail = screen
      .getByRole("button", { name: "Skip for now" })
      .closest(".ob-stage-acts");
    expect(rail).not.toBeNull();
    expect(
      within(rail as HTMLElement).getByText(en["ob.conv.connect.skipCaption"]),
    ).toBeInTheDocument();
  });

  it("records a skip as connect_skipped", async () => {
    stub();
    const { persist, dispatch } = renderAct();
    const user = userEvent.setup();
    const skip = screen.getByRole("button", { name: "Skip for now" });
    await waitFor(() => expect(skip).not.toBeDisabled());
    await user.click(skip);

    await waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "CONNECT_DONE" }),
    );
    expect(persist).toHaveBeenCalledWith(
      expect.objectContaining({ step: "complete", connectSkipped: true }),
    );
  });
});

describe("coming back from a refused consent", () => {
  it("reopens the account this tab's own trip was for, ready to ask again", async () => {
    sessionStorage.setItem("ob.connect.oauthAttempt", "gcal");
    stub();
    renderAct("denied", "gcal");

    expect(
      await screen.findByText(en["ob.conv.connect.calendarFailedTitle"]),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("radio", { name: /Google/ })).toBeChecked(),
    );
    expect(
      screen.getByRole("button", { name: "Connect mail and calendar" }),
    ).toBeInTheDocument();
    // Spent on read: a reload of the same address is the stale link it is.
    expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBeNull();
  });

  it("picks nothing for a stale link this tab never started", async () => {
    stub();
    renderAct("denied", "gmail");

    expect(
      await screen.findByText(en["ob.s4.connectDenied"]),
    ).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /Google/ })).not.toBeChecked();
    expect(
      screen.queryByRole("button", { name: "Connect mail and calendar" }),
    ).toBeNull();
  });
});
