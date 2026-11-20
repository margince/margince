// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { parseHash } from "../lib/router";
import { readStored, STORAGE_KEYS } from "../lib/storage";
import { FIXTURE_TIMEZONE, meFixture } from "../testing/mefixture";
import {
  ignoreSearch,
  newClient,
  renderWith,
  stubPhoneViewport,
} from "../testing/shellharness";
import { TopBar } from "./topbar";

// The three personal steps every seat is asked: where it works (its time
// zone), its language, and what Margince may do. A member invited to an
// existing workspace holds no connection or voice grant, so these are all the
// checklist asks of it.

type Me = ReturnType<typeof meFixture>;

function seat(user: Partial<Me["user"]> = {}): Me {
  const me = meFixture({ roles: ["rep"], allow: {} });
  return { ...me, user: { ...me.user, ...user } };
}

function mount(me: Me) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("{}", { status: 404 })),
  );
  const client = newClient();
  client.setQueryData(["me"], me);
  return renderWith(
    client,
    <TopBar
      route={parseHash("#/deals")}
      collapsed={false}
      onOpenSearch={ignoreSearch}
    />,
  );
}

const trigger = () => screen.findByRole("button", { name: /Finish setup/ });

async function openPanel(me: Me) {
  mount(me);
  const user = userEvent.setup();
  await user.click(await trigger());
  const panel = screen.getByRole("region", { name: /Finish setup/ });
  return { user, panel };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  window.location.hash = "";
});

describe("the personal setup steps", () => {
  it("asks a member with no connection grant for the three personal steps only", async () => {
    mount(seat());

    expect((await trigger()).textContent).toContain("Finish setup0 of 3");
  });

  it("reads each step's state from the seat the server answered", async () => {
    const { panel } = await openPanel(
      seat({ timezone: FIXTURE_TIMEZONE, timezone_chosen: true }),
    );

    const steps = within(panel).getAllByRole("listitem");
    expect(
      steps.map((step) => step.querySelector(".t-name")?.textContent),
    ).toMatchInlineSnapshot(`
      [
        "Choose where you work Done",
        "Choose your language Still to do",
        "See what Margince may do Still to do",
      ]
    `);
    expect(steps[0].textContent).toContain(`Times follow ${FIXTURE_TIMEZONE}.`);
  });

  it("keeps a zone the browser guessed or an agent set as still to choose", async () => {
    const { panel } = await openPanel(seat({ timezone_chosen: false }));

    const first = within(panel).getAllByRole("listitem")[0];
    expect(first.textContent).toContain("Still to do");
    expect(first.getAttribute("aria-current")).toBe("step");
  });

  it("counts a language the seat chose as done", async () => {
    mount(seat({ locale: "de" }));

    expect((await trigger()).textContent).toContain("Finish setup1 of 3");
  });

  it("sends the time zone and language steps to the Account card that holds them", async () => {
    const { user, panel } = await openPanel(seat());

    await user.click(
      within(panel).getByRole("button", { name: "Choose place" }),
    );
    expect(window.location.hash).toBe(
      "#/settings/account?focus=account-language",
    );

    window.location.hash = "";
    await user.click(
      within(panel).getByRole("button", { name: "Choose language" }),
    );
    expect(window.location.hash).toBe(
      "#/settings/account?focus=account-language",
    );
  });

  it("explains what Margince reads and writes, and finishes the step only on Got it", async () => {
    const { user, panel } = await openPanel(seat());
    const owner = seat().user.id;

    await user.click(within(panel).getByRole("button", { name: "Review" }));
    const dialog = await screen.findByRole("dialog", {
      name: "What Margince reads and writes",
    });
    expect(within(dialog).getAllByRole("listitem")).toHaveLength(3);

    await user.click(within(dialog).getByRole("button", { name: "Later" }));
    expect(
      readStored({
        family: STORAGE_KEYS.setupPermissionsSeen,
        member: owner,
      }),
    ).toBeNull();

    await user.click(within(panel).getByRole("button", { name: "Review" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Got it",
      }),
    );
    expect(
      readStored({
        family: STORAGE_KEYS.setupPermissionsSeen,
        member: owner,
      }),
    ).toBe("1");
    expect((await trigger()).textContent).toContain("1 of 3");
    // The button became a badge; focus must stay inside the checklist.
    expect(document.activeElement).not.toBe(document.body);
    expect(panel.contains(document.activeElement)).toBe(true);
  });

  it("states no absolute the product breaks", async () => {
    const { user, panel } = await openPanel(seat());
    await user.click(within(panel).getByRole("button", { name: "Review" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).not.toMatch(/\bonly\b|nothing is sent/i);
    expect(dialog.textContent).toContain("password resets");
  });

  it("adds no setup row to the wide account menu", async () => {
    mount(seat());
    await trigger();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: /account$/i }));
    await screen.findByRole("menu", { name: "Account" });
    expect(screen.queryByRole("menuitem", { name: /Finish setup/ })).toBeNull();
  });

  it("stands down once the member chose a zone and a language and read the permissions", async () => {
    const { user, panel } = await openPanel(seat());
    await user.click(within(panel).getByRole("button", { name: "Review" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Got it",
      }),
    );
    cleanup();

    mount(seat({ timezone_chosen: true, locale: "vi" }));
    await vi.waitFor(() =>
      expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull(),
    );
  });

  it("hides until the next sign-in for a member too", async () => {
    const { user, panel } = await openPanel(seat());

    await user.click(
      within(panel).getByRole("button", { name: "Hide until next sign-in" }),
    );

    expect(screen.queryByRole("button", { name: /Finish setup/ })).toBeNull();
  });

  it("opens the account menu's setup row on the card of the next personal step", async () => {
    stubPhoneViewport();
    mount(seat());
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /account$/i }));

    const row = await screen.findByRole("menuitem", { name: /Finish setup/ });
    expect(row.getAttribute("href")).toBe(
      "/#/settings/account?focus=account-language",
    );
  });
});
