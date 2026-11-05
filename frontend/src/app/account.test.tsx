// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { LocaleProvider } from "../i18n";
import { STORAGE_KEYS } from "../lib/storage";
import { setThemeChoice } from "../lib/theme";
import { bookingProfile } from "../testing/book.testkit";
import { meFixture } from "../testing/mefixture";
import { AccountMenu } from "./account";

const THEME_KEY = STORAGE_KEYS.theme.name;

// The account block: a glyph in the top bar's trail. Its menu is the one door
// into settings and the one appearance control. Behaviour only: where the panel
// opens is CSS, but what it holds and what a keyboard reader can do are promises.

// The theme is ONE module-level store shared by every mounted control, so it
// outlives a case. Every case here starts from the unchosen state — following
// the machine, which the suite's matchMedia stub answers with light.
beforeEach(() => {
  setThemeChoice("system");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.localStorage.removeItem(THEME_KEY);
});

// Seeded through the cache rather than a fetch stub: `useMe` reads ["me"], and
// a fixture written into it is the same snapshot the shell renders from.
const render = (ui: Parameters<typeof rtlRender>[0]) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture());
  return rtlRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    ),
  });
};

// The same shell, with this reader carrying a different display name and the
// same address — which is what a rename is.
const renderNamed = (displayName: string) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const me = meFixture();
  client.setQueryData(["me"], {
    ...me,
    user: { ...me.user, display_name: displayName },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AccountMenu />
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

// Expanded, the trigger's accessible name carries the visible name as well as
// what the control does, so it is matched by suffix rather than by equality.
// Glyph-only there is no visible text to carry, and the name is exact.
const railTrigger = () => screen.getByRole("button", { name: /Account$/ });
const glyphTrigger = () => screen.getByRole("button", { name: "Account" });

const openMenu = async (user: UserEvent, trigger: HTMLElement) => {
  await user.click(trigger);
  return trigger;
};

const row = (name: string) => screen.getByRole("menuitem", { name });
const head = () =>
  screen.getByRole("menuitem", {
    name: "Test User — test@example.test: account settings",
  });
const choice = (name: string) => screen.getByRole("radio", { name });

describe("AccountMenu", () => {
  // The rail's chip and the settings page's chip are the SAME contact, so they
  // wear the same mesh — and keep it when the display name changes, because
  // the mesh is keyed on the address rather than on the name.
  it("keys the chip's mesh on the address, not the display name", async () => {
    const user = userEvent.setup();
    const meshOf = async (root: HTMLElement) => {
      await openMenu(user, glyphTrigger());
      return root
        .querySelector(".acctmenuwho .avatar-mesh")
        ?.getAttribute("style");
    };
    const { container: named } = renderNamed("Test User");
    const mesh = await meshOf(named);
    expect(mesh).toContain("--avatar-hue-a");
    cleanup();
    const { container: renamed } = renderNamed("Renamed Contact");
    expect(await meshOf(renamed)).toBe(mesh);
  });

  // WCAG 2.5.3: the row prints the contact's name, so a voice user who says the
  // word they can read has to reach this control.
  it("opens on the trigger and closes on it again", async () => {
    const user = userEvent.setup();
    const { container } = render(<AccountMenu />);
    expect(container.querySelector(".accountmenu")).toBeNull();

    const trigger = await openMenu(user, railTrigger());
    expect(screen.getByRole("menu", { name: /Account/ })).toBeTruthy();
    expect(trigger.getAttribute("aria-haspopup")).toBe("menu");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");

    await user.click(trigger);
    expect(container.querySelector(".accountmenu")).toBeNull();
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  // ONE door into settings, and it is this menu. The sidebar's foot used to
  // carry the other one; it is gone, so a reader who cannot open this menu
  // cannot reach the settings level at all — which is why the door is asserted
  // here rather than merely permitted.
  it("holds the product's one settings door", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());

    expect(row("Settings").getAttribute("href")).toBe("/#/settings");
    // Exactly one: a second row a click apart landing on the same level is the
    // shape this menu replaced.
    expect(
      screen
        .getAllByRole("menuitem")
        .filter((item) => item.getAttribute("href") === "/#/settings"),
    ).toHaveLength(1);
  });

  // The head opens the seat's own page, not the settings level. Its name says
  // whose account it is and where it leads, and the menu goes with the click.
  it("opens the seat's own account page from its head", async () => {
    const user = userEvent.setup();
    const { container } = render(<AccountMenu />);
    await openMenu(user, railTrigger());

    expect(head().getAttribute("href")).toBe("/#/settings/account");
    expect(head().classList).toContain("acctrow");
    await user.click(head());
    expect(container.querySelector(".accountmenu")).toBeNull();
  });

  // The booking link row carries the guest-facing state: a page switched off,
  // or one whose calendar cannot take bookings, is paused for every guest.
  it.each([
    [{ enabled: false, blocker: "none" }, true],
    [{ enabled: true, blocker: "calendar_read_only" }, true],
    [{ enabled: true, blocker: "none" }, false],
  ] as const)(
    "marks the booking link paused for %o: %s",
    async (state, paused) => {
      const user = userEvent.setup();
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      client.setQueryData(["me"], meFixture());
      client.setQueryData(["scheduling-profile"], {
        ...bookingProfile,
        ...state,
      });
      rtlRender(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">
            <AccountMenu />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      await openMenu(user, railTrigger());
      const link = screen.getByRole("menuitem", { name: /My booking link/ });
      expect(link.textContent?.includes("Paused")).toBe(paused);
    },
  );

  // The appearance choice IS a destination now: it lives where the reader can
  // see the document repaint under the open panel, rather than on a form two
  // navigations away that they then have to navigate back out of.
  it("owns the appearance choice, behind a submenu of its own", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());

    const theme = row("Theme");
    // No popup claim: what the row opens is a group of radios, and every value
    // `aria-haspopup` takes names a widget this is not.
    expect(theme.getAttribute("aria-haspopup")).toBeNull();
    expect(theme.getAttribute("aria-expanded")).toBe("false");
    // Closed, the choices are not merely hidden — they are not rendered, so
    // there is nothing behind the row for a Tab or a screen reader to find.
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
  });

  it("opens the theme submenu from the pointer", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());

    await user.click(row("Theme"));
    expect(row("Theme").getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("group", { name: "Theme" })).toBeTruthy();
    expect(
      screen
        .getAllByRole("radio")
        .map((item) => item.closest("label")?.textContent),
    ).toEqual(["Light", "Dark", "System"]);
  });

  // Right opens it and lands the reader on the choice they already have, so the
  // submenu is usable by somebody who never touches a pointer — which is the
  // whole reason this panel became a real menu.
  it("opens the theme submenu from the keyboard, on the standing choice", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());

    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(document.activeElement).toBe(row("Theme"));

    await user.keyboard("{ArrowRight}");
    expect(row("Theme").getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(choice("System"));
  });

  // Through to the real store, which is the only thing that can prove the tick
  // reports the document rather than decorating the row.
  it("writes the picked appearance through the theme store", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());
    await user.click(row("Theme"));

    const system = choice("System");
    const dark = choice("Dark");
    expect(system).toBeChecked();
    expect(dark).not.toBeChecked();

    await user.click(dark);

    expect(choice("Dark")).toBeChecked();
    expect(choice("System")).not.toBeChecked();
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(window.localStorage.getItem(THEME_KEY)).toBe("dark");
    // The group stays open on a pick: the whole of the feedback is the document
    // repainting under it with the tick on the answer that did it.
    expect(screen.getByRole("group", { name: "Theme" })).toBeTruthy();
  });

  // One keystroke, one layer. The dismissal both layers share stands the outer
  // one down while the row that opened the inner one still reads expanded
  // (app/popover.ts), so a reader is never taken two steps back by one press.
  it("closes the submenu on Escape, then the menu, returning focus each time", async () => {
    const user = userEvent.setup();
    const { container } = render(<AccountMenu />);
    const trigger = await openMenu(user, railTrigger());
    await user.click(row("Theme"));
    expect(document.activeElement).toBe(choice("System"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("group", { name: "Theme" })).toBeNull();
    expect(container.querySelector(".accountmenu")).not.toBeNull();
    expect(document.activeElement).toBe(row("Theme"));
    expect(row("Theme").getAttribute("aria-expanded")).toBe("false");

    await user.keyboard("{Escape}");
    expect(container.querySelector(".accountmenu")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  // Left is the other way back out, and it is the one a reader who walked in
  // with Right reaches for.
  it("walks back out of the submenu with Left", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());
    await user.click(row("Theme"));

    await user.keyboard("{ArrowLeft}");
    expect(screen.queryByRole("menu", { name: "Theme" })).toBeNull();
    expect(document.activeElement).toBe(row("Theme"));
  });

  it("walks its rows with Up and Down, and its ends with Home and End", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());
    // The menu takes focus when it opens, or a keyboard reader has to walk into
    // it with a key nothing told them about. The head is its first row.
    expect(document.activeElement).toBe(head());

    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("Settings"));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("Theme"));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("My booking link"));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("Help and feedback"));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("Sign out"));
    // Wrapping, so the walk has no dead end.
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(head());

    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(row("Sign out"));
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(head());
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(row("Sign out"));
  });

  // A roving tabstop: exactly one row is in the tab order at a time, so Tab
  // leaves the menu rather than walking it.
  it("keeps one tabstop among the rows, on whichever holds focus", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());

    const stops = () =>
      screen
        .getAllByRole("menuitem")
        .filter((item) => item.getAttribute("tabindex") === "0");
    expect(stops()).toEqual([head()]);

    await user.keyboard("{ArrowDown}");
    expect(stops()).toEqual([row("Settings")]);
  });

  // A dialog raised over the open menu owns Escape: one press closes the
  // dialog alone, not it and the menu the reader cannot see under it.
  it("leaves Escape to a dialog raised over it", async () => {
    const onDialogClose = vi.fn();
    const page = (dialogOpen: boolean) => (
      <>
        <AccountMenu />
        <Modal
          open={dialogOpen}
          onClose={onDialogClose}
          labelledBy="edit"
          intent="form"
        >
          <Heading size="large" id="edit">
            Edit deal
          </Heading>
        </Modal>
      </>
    );
    const user = userEvent.setup();
    const { container, rerender } = render(page(false));
    await openMenu(user, railTrigger());
    rerender(page(true));

    await user.keyboard("{Escape}");

    expect(onDialogClose).toHaveBeenCalledOnce();
    expect(container.querySelector(".accountmenu")).not.toBeNull();
  });

  it("hands focus back to the trigger when Escape closes it", async () => {
    const user = userEvent.setup();
    const { container } = render(<AccountMenu />);
    const trigger = await openMenu(user, railTrigger());
    // Standing on a row, the way a keyboard user arrives at one.
    const signOut = row("Sign out");
    signOut.focus();
    expect(document.activeElement).toBe(signOut);

    await user.keyboard("{Escape}");
    expect(container.querySelector(".accountmenu")).toBeNull();
    // Not document.body: dismissing unmounts the focused row, and focus left on
    // the body restarts the next Tab at the top of the page, having lost the
    // chrome the user was standing in.
    expect(document.activeElement).toBe(trigger);
  });

  it("keeps the way out reachable from the menu", async () => {
    const user = userEvent.setup();
    render(<AccountMenu />);
    await openMenu(user, railTrigger());
    expect(row("Sign out").hasAttribute("disabled")).toBe(false);
  });

  // The row re-enables when the request settles either way, so a session that is
  // still open would otherwise look exactly like one that has ended — and the
  // next thing the reader does, they do believing they signed out. The server's
  // own words, in an alert, in the menu they asked from.
  it("says so when signing out is refused, in the server's own words", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", async () =>
      Response.json(
        { title: "Conflict", detail: "Your session is already gone." },
        {
          status: 409,
          headers: { "content-type": "application/problem+json" },
        },
      ),
    );
    render(<AccountMenu />);
    await openMenu(user, railTrigger());
    await user.click(row("Sign out"));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Your session is already gone.");
  });

  // Closed, the panel is not rendered at all — so there is nothing behind the
  // trigger for Tab to land on, and nothing for a screen reader to read out of
  // a menu that is not open.
  it("puts nothing in the tab order while it is closed", () => {
    const { container } = render(<AccountMenu />);
    expect(container.querySelectorAll("a, button")).toHaveLength(1);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  // The trigger prints nothing, and a glyph alone tells a screen reader
  // nothing. The sentence is carried as clipped text and wired as the trigger's
  // description — the `title` is the pointer's copy of it and is never the
  // accessible name.
  it("carries the identity for a screen reader without printing it", () => {
    const { container } = render(<AccountMenu />);
    const trigger = glyphTrigger();
    expect(container.querySelector(".acctwho")).toBeNull();

    const describedBy = trigger.getAttribute("aria-describedby") ?? "";
    const spoken = document.getElementById(describedBy);
    expect(spoken?.className).toContain("sr-only");
    expect(spoken?.textContent).toContain("Test User");
    expect(spoken?.textContent).toContain("test@example.test");
    expect(trigger.getAttribute("title")).toBe(spoken?.textContent);
  });

  // The trigger is a glyph alone, so its name is all that identifies it. The
  // panel is where the reader confirms whose account they are about to act on.
  it("names the top bar's glyph-only trigger, and prints the identity in the panel", async () => {
    const user = userEvent.setup();
    const { container } = render(<AccountMenu />);
    const trigger = glyphTrigger();
    expect(container.querySelector(".acctwho")).toBeNull();
    expect(trigger.getAttribute("aria-label")).toBe("Account");
    expect(trigger.querySelector(".avatar")).toBeNull();

    await openMenu(user, trigger);
    const who = container.querySelector(".acctmenuwho");
    // The chip stands before the two lines, in the panel only.
    expect(who?.firstElementChild?.classList).toContain("avatar");
    expect(who?.querySelector("b")?.textContent).toBe("Test User");
    expect(who?.querySelector(".acctmail")?.textContent).toBe(
      "test@example.test",
    );
    // The menu is named by what it is, as the trigger is. The head row names
    // whose account it is, and focus lands on it.
    expect(screen.getByRole("menu", { name: "Account" })).toBe(
      container.querySelector(".accountmenu"),
    );
    expect(document.activeElement).toBe(head());
  });
});

// A session whose /me names nobody yet: no head row, and focus starts on
// Settings. The client is returned, so a case can let /me answer later.
const renderHeadless = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const me = meFixture();
  client.setQueryData(["me"], {
    ...me,
    user: { ...me.user, display_name: "", email: "" },
  });
  // The mount's own refetch never answers, so only the case changes /me.
  vi.stubGlobal("fetch", () => new Promise(() => undefined));
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AccountMenu />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
};

describe("AccountMenu before /me names the seat", () => {
  it("starts on Settings and walks only the rows it draws", async () => {
    const user = userEvent.setup();
    renderHeadless();
    await openMenu(user, glyphTrigger());
    expect(screen.queryByRole("menuitem", { name: /account settings/ })).toBe(
      null,
    );
    expect(document.activeElement).toBe(row("Settings"));

    const walked = [];
    for (let step = 0; step < 5; step++) {
      await user.keyboard("{ArrowDown}");
      walked.push(document.activeElement?.textContent);
    }
    expect(walked).toEqual([
      "Theme",
      "My booking link",
      "Help and feedback",
      "Sign out",
      "Settings",
    ]);
    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(row("Sign out"));
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(row("Settings"));
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(row("Sign out"));
  });

  it("hands focus to the head when /me answers while focus is on Settings", async () => {
    const user = userEvent.setup();
    const client = renderHeadless();
    await openMenu(user, glyphTrigger());
    expect(document.activeElement).toBe(row("Settings"));

    act(() => client.setQueryData(["me"], meFixture()));
    await waitFor(() => expect(document.activeElement).toBe(head()));
  });

  it("leaves focus on the row the reader walked to when /me answers", async () => {
    const user = userEvent.setup();
    const client = renderHeadless();
    await openMenu(user, glyphTrigger());
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(row("Theme"));

    act(() => client.setQueryData(["me"], meFixture()));
    await screen.findByRole("menuitem", { name: /account settings/ });
    expect(document.activeElement).toBe(row("Theme"));
  });

  it("leaves focus on Settings when /me answers after the reader walked back to it", async () => {
    const user = userEvent.setup();
    const client = renderHeadless();
    await openMenu(user, glyphTrigger());
    await user.keyboard("{ArrowDown}{ArrowUp}");
    expect(document.activeElement).toBe(row("Settings"));

    act(() => client.setQueryData(["me"], meFixture()));
    await screen.findByRole("menuitem", { name: /account settings/ });
    expect(document.activeElement).toBe(row("Settings"));
  });
});
