// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { parseHash } from "../lib/router";
import {
  forgetSeat,
  readStored,
  STORAGE_KEYS,
  writeStored,
} from "../lib/storage";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  ignoreSearch,
  newClient,
  renderWith,
  stubPhoneViewport,
} from "../testing/shellharness";
import { TopBar } from "./topbar";

// The top bar's "Finish setup" chip: what it counts, what opening it shows, and
// the three ways it stands down. Those are finished, hidden for this sign-in,
// and nothing to ask.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const VOICE_WRITER: GrantSpec = {
  voice_profile: ["read", "create", "update"],
  activity: ["read", "create"],
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function gmail(): CaptureConnection {
  return {
    id: "00000000-0000-4000-8000-000000000011",
    provider: "gmail",
    can_backfill: true,
    status: "connected",
    scopes: [],
    account_label: "ada@example.test",
  };
}

type World = Readonly<{
  rows?: readonly CaptureConnection[];
  providers?: readonly { provider: string; reason: string }[];
  voiceVersion?: number;
  bookingOn?: boolean;
  /** Every read waits for it, so a test can answer them while a menu is open. */
  answersAfter?: Promise<void>;
}>;

function serve(world: World = {}) {
  const answers: Readonly<Record<string, () => Response>> = {
    "/connectors": () =>
      jsonResponse({
        data: world.rows ?? [gmail()],
        providers: world.providers ?? [
          { provider: "gmail", reason: "ready" },
          { provider: "graph", reason: "ready" },
          { provider: "imap", reason: "ready" },
        ],
      }),
    "/voice-profiles": () =>
      jsonResponse(
        world.voiceVersion
          ? { data: [{ profile_version: world.voiceVersion }] }
          : { data: [] },
      ),
    "/scheduling/profile": () =>
      jsonResponse({
        enabled: world.bookingOn ?? false,
        provider: "",
        calendar_id: "",
        duration_minutes: 30,
        notice_minutes: 240,
        horizon_days: 30,
        buffer_minutes: 0,
        title: "Intro call",
        location: "",
      }),
  };
  vi.stubGlobal(
    "fetch",
    // The connector discovery read asks by URL string, the API client by Request.
    vi.fn(async (request: Request | string) => {
      await world.answersAfter;
      const url = typeof request === "string" ? request : request.url;
      const path = new URL(url, "https://crm.test").pathname;
      const route = Object.keys(answers).find((end) => path.endsWith(end));
      return route ? answers[route]() : jsonResponse({}, 404);
    }),
  );
}

// A reader who already answered the three personal steps, so the cases about
// mail, calendar, voice and booking keep their own shape.
function mountTopBar(hash = "#/deals", personal: "done" | "open" = "done") {
  const client = newClient();
  const me = meFixture({
    allow: VOICE_WRITER,
    personalChoicesMade: personal === "done",
  });
  client.setQueryData(["me"], me);
  if (personal === "done") {
    writeStored(
      { family: STORAGE_KEYS.setupPermissionsSeen, member: me.user.id },
      "1",
    );
  }
  const view = renderWith(
    client,
    <TopBar route={parseHash(hash)} onOpenSearch={ignoreSearch} />,
  );
  const rerouted = (next: string, held = false) =>
    view.rerender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <TopBar
            route={parseHash(next)}
            onOpenSearch={ignoreSearch}
            held={held}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  // Every read in the bar has answered or failed, so an absent chip is final.
  const answered = () =>
    vi.waitFor(() =>
      expect(
        client
          .getQueryCache()
          .getAll()
          .every((query) => query.state.fetchStatus === "idle"),
      ).toBe(true),
    );
  return { ...view, rerouted, answered };
}

const trigger = () => screen.findByRole("button", { name: /Finish setup/ });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  window.location.hash = "";
});

describe("SetupReminder", () => {
  it("counts the finished steps and opens the checklist with the next one first", async () => {
    serve();
    mountTopBar();

    const chip = await trigger();
    expect(chip.textContent).toContain("Finish setup4 of 7");
    await userEvent.setup().click(chip);

    const panel = screen.getByRole("region", { name: /Finish setup/ });
    within(panel).getByRole("heading", {
      name: englishCopy["setup.reminder.title"],
    });
    const steps = within(panel).getAllByRole("listitem");
    expect(
      steps.map((step) => step.querySelector(".t-name")?.textContent),
    ).toMatchInlineSnapshot(`
      [
        "Connect your mail Done",
        "Connect your calendar Still to do",
        "Choose where you work Done",
        "Choose your language Done",
        "See what Margince may do Done",
        "Train your writing voice Still to do",
        "Turn on your booking link ",
      ]
    `);
    expect(steps[0].textContent).toContain("ada@example.test, Google");
    expect(steps[1].getAttribute("aria-current")).toBe("step");
    expect(steps[1].textContent).toContain(
      englishCopy["setup.caption.calendarSkipped"],
    );
    // The booking link waits on the calendar: it explains, wears the grey
    // Waiting badge and offers no verb.
    expect(steps[6].textContent).toContain(
      englishCopy["setup.caption.bookingNeedsCalendar"],
    );
    expect(within(steps[6]).queryByRole("button")).toBeNull();
    expect(steps[6].querySelector(".badge")?.textContent).toBe("Waiting");
    // The current step's verb is primary; the other open ones are not.
    expect(
      within(panel)
        .getAllByRole("button", { name: /^Connect|^Add/ })
        .map((verb) => verb.className.includes("btn-primary")),
    ).toEqual([true, false]);
    within(panel).getByText("4 of 7 done");
    expect(
      within(panel)
        .getByRole("meter", { name: englishCopy["setup.reminder.progress"] })
        .getAttribute("aria-valuenow"),
    ).toBe("4");
  });

  it("sends a step's action to its settings page", async () => {
    serve();
    mountTopBar();
    const user = userEvent.setup();
    await user.click(await trigger());

    await user.click(
      screen.getByRole("button", {
        name: englishCopy["setup.action.calendar"],
      }),
    );
    expect(window.location.hash).toBe("#/settings/connections");
  });

  it("asks to reconnect an expired calendar instead of calling it skipped", async () => {
    serve({
      rows: [
        gmail(),
        {
          ...gmail(),
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          status: "reauth_required",
        },
      ],
    });
    mountTopBar();
    await userEvent.setup().click(await trigger());

    const steps = within(
      screen.getByRole("region", { name: /Finish setup/ }),
    ).getAllByRole("listitem");
    expect(steps[1].textContent).toContain(
      englishCopy["setup.caption.calendarReconnect"],
    );
    const reconnect = within(steps[1]).getByRole("button");
    expect(reconnect.textContent).toBe("Reconnect");
    expect(reconnect.className).toContain("btn-primary");
  });

  it("closes the panel when the route changes under it", async () => {
    serve();
    const view = mountTopBar();
    await userEvent.setup().click(await trigger());
    expect(
      screen.queryByRole("region", { name: /Finish setup/ }),
    ).not.toBeNull();

    view.rerouted("#/settings/connections");
    expect((await trigger()).getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region", { name: /Finish setup/ })).toBeNull();
  });

  it("gives way to Back to setup while onboarding holds the reader", async () => {
    serve();
    const view = mountTopBar();
    await trigger();

    view.rerouted("#/deals", true);

    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
    expect(
      screen.getByRole("link", { name: englishCopy["shell.backToSetup"] }),
    ).toBeTruthy();
  });

  it("hides until the next sign-in, and comes back after it", async () => {
    serve();
    const first = mountTopBar();
    const user = userEvent.setup();
    await user.click(await trigger());
    await user.click(
      screen.getByRole("button", { name: englishCopy["setup.reminder.hide"] }),
    );

    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
    // Focus stays in the top bar rather than falling to the page body.
    expect(document.activeElement).toBe(
      screen.getByRole("button", {
        name: englishCopy["shell.searchEverything"],
      }),
    );
    expect(readStored(STORAGE_KEYS.setupReminderHidden)).toBe("1");
    first.unmount();

    forgetSeat();
    mountTopBar();
    expect((await trigger()).textContent).toContain("4 of 7");
  });

  it("stands down once every step is done", async () => {
    serve({
      rows: [
        gmail(),
        {
          ...gmail(),
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      voiceVersion: 2,
      bookingOn: true,
    });
    await mountTopBar().answered();
    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
  });

  it("asks nothing where no step can be shown", async () => {
    serve({
      rows: [],
      providers: [
        { provider: "gmail", reason: "unsupported" },
        { provider: "graph", reason: "unsupported" },
        { provider: "imap", reason: "unsupported" },
      ],
      voiceVersion: 2,
      bookingOn: true,
    });
    await mountTopBar().answered();
    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
  });
});

describe("SetupReminder at phone width", () => {
  const openAccount = async (user: UserEvent) => {
    await user.click(screen.getByRole("button", { name: /account$/i }));
    await screen.findByRole("menu", { name: "Account" });
    return screen.getAllByRole("menuitem").map((row) => row.textContent ?? "");
  };

  it("moves from the bar into the account menu, after Help and feedback", async () => {
    stubPhoneViewport();
    serve();
    await mountTopBar().answered();
    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();

    await openAccount(userEvent.setup());
    await screen.findByRole("menuitem", { name: /Finish setup/ });
    const rows = screen
      .getAllByRole("menuitem")
      .map((row) => row.textContent ?? "");
    expect(rows.indexOf("Finish setup4 of 7")).toBe(
      rows.indexOf(englishCopy["shell.helpMenu"]) + 1,
    );
    // The row leads where the panel's current step would.
    expect(
      screen
        .getByRole("menuitem", { name: /Finish setup/ })
        .getAttribute("href"),
    ).toBe("/#/settings/connections");
  });

  it("stays out of the account menu on a wide screen", async () => {
    serve();
    await mountTopBar().answered();
    expect(await trigger()).toBeTruthy();
    expect((await openAccount(userEvent.setup())).join()).not.toContain(
      englishCopy["setup.reminder.trigger"],
    );
  });

  it("stays out of the account menu after a hide on a wide screen", async () => {
    const viewport = stubPhoneViewport({ wide: true });
    serve();
    const view = mountTopBar();
    const user = userEvent.setup();
    await user.click(await trigger());
    await user.click(
      screen.getByRole("button", { name: englishCopy["setup.reminder.hide"] }),
    );
    act(() => viewport.narrow());
    await view.answered();
    expect((await openAccount(user)).join()).not.toContain(
      englishCopy["setup.reminder.trigger"],
    );
  });

  it("keeps focus and the tabstop on Sign out when the row arrives under it", async () => {
    stubPhoneViewport();
    let answer = () => {};
    serve({ answersAfter: new Promise((resolve) => (answer = resolve)) });
    const view = mountTopBar();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /account$/i }));
    // The menu draws from lazy chunks, so it is waited on; an earlier case in
    // the file may or may not have loaded them.
    await screen.findByRole("menu", { name: "Account" });
    await user.keyboard("{End}");
    const signOut = screen.getByRole("menuitem", { name: "Sign out" });
    expect(document.activeElement).toBe(signOut);

    answer();
    await view.answered();
    await screen.findByRole("menuitem", { name: /Finish setup/ });
    expect(document.activeElement).toBe(signOut);
    expect(signOut.tabIndex).toBe(0);
    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(
      screen.getByRole("menuitem", { name: /Finish setup/ }),
    );
  });

  it("closes the account menu onto its trigger when the focused row is hidden elsewhere", async () => {
    stubPhoneViewport();
    serve();
    await mountTopBar().answered();
    const user = userEvent.setup();
    const account = screen.getByRole("button", { name: /account$/i });
    await user.click(account);
    await screen.findByRole("menu", { name: "Account" });
    await user.keyboard("{End}{ArrowUp}");
    expect(document.activeElement).toBe(
      screen.getByRole("menuitem", { name: /Finish setup/ }),
    );

    // Another tab's hide reaches this one through the stored flag.
    act(() => writeStored(STORAGE_KEYS.setupReminderHidden, "1"));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(account);
  });

  it("leaves the account menu once every step is done", async () => {
    stubPhoneViewport();
    serve({
      rows: [
        gmail(),
        {
          ...gmail(),
          id: "00000000-0000-4000-8000-000000000012",
          provider: "gcal",
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
        },
      ],
      voiceVersion: 2,
      bookingOn: true,
    });
    await mountTopBar().answered();
    expect((await openAccount(userEvent.setup())).join()).not.toContain(
      englishCopy["setup.reminder.trigger"],
    );
  });
});
