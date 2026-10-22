// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { parseHash } from "../lib/router";
import { forgetSeat, readStored, STORAGE_KEYS } from "../lib/storage";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { ignoreSearch, newClient, renderWith } from "../testing/shellharness";
import { TopBar } from "./topbar";

// The top bar's "Finish setup" chip: what it counts, what opening it shows, and
// the three ways it stands down. Those are finished, hidden for this sign-in,
// and nothing to ask.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const VOICE_WRITER: GrantSpec = { voice_profile: ["read", "create", "update"] };

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
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const route = Object.keys(answers).find((end) => path.endsWith(end));
      return route ? answers[route]() : jsonResponse({}, 404);
    }),
  );
}

function mountTopBar(hash = "#/deals") {
  const client = newClient();
  client.setQueryData(["me"], meFixture({ allow: VOICE_WRITER }));
  const view = renderWith(
    client,
    <TopBar
      route={parseHash(hash)}
      collapsed={false}
      onOpenSearch={ignoreSearch}
    />,
  );
  const rerouted = (next: string) =>
    view.rerender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <TopBar
            route={parseHash(next)}
            collapsed={false}
            onOpenSearch={ignoreSearch}
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
    expect(chip.textContent).toContain("Finish setup1 of 4");
    await userEvent.setup().click(chip);

    const panel = screen.getByRole("region", { name: /Finish setup/ });
    within(panel).getByRole("heading", { name: "Finish setting up" });
    const steps = within(panel).getAllByRole("listitem");
    expect(
      steps.map((step) => step.querySelector(".t-name")?.textContent),
    ).toMatchInlineSnapshot(`
      [
        "Connect your mail Done",
        "Connect your calendar Still to do",
        "Train your writing voice Still to do",
        "Turn on your booking link ",
      ]
    `);
    expect(steps[0].textContent).toContain("ada@example.test, Google");
    expect(steps[1].getAttribute("aria-current")).toBe("step");
    expect(steps[1].textContent).toContain("Skipped during setup.");
    // The booking link waits on the calendar: it explains, wears the grey
    // Waiting badge and offers no verb.
    expect(steps[3].textContent).toContain("Needs a calendar.");
    expect(within(steps[3]).queryByRole("button")).toBeNull();
    expect(steps[3].querySelector(".badge")?.textContent).toBe("Waiting");
    // The current step's verb is primary; the other open ones are not.
    expect(
      within(panel)
        .getAllByRole("button", { name: /^Connect|^Add/ })
        .map((verb) => verb.className.includes("btn-primary")),
    ).toEqual([true, false]);
    within(panel).getByText("1 of 4 done");
    expect(
      within(panel)
        .getByRole("meter", { name: "Setup progress" })
        .getAttribute("aria-valuenow"),
    ).toBe("1");
  });

  it("sends a step's action to its settings page", async () => {
    serve();
    mountTopBar();
    const user = userEvent.setup();
    await user.click(await trigger());

    await user.click(screen.getByRole("button", { name: "Connect calendar" }));
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
    expect(steps[1].textContent).toContain("Expired or cannot add bookings.");
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

  it("hides until the next sign-in, and comes back after it", async () => {
    serve();
    const first = mountTopBar();
    const user = userEvent.setup();
    await user.click(await trigger());
    await user.click(
      screen.getByRole("button", { name: "Hide until next sign-in" }),
    );

    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
    // Focus stays in the top bar rather than falling to the page body.
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Search or ask Margince" }),
    );
    expect(readStored(STORAGE_KEYS.setupReminderHidden)).toBe("1");
    first.unmount();

    forgetSeat();
    mountTopBar();
    expect((await trigger()).textContent).toContain("1 of 4");
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
