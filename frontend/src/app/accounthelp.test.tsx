// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { useShortcutDispatcher } from "../lib/shortcuts";
import { meFixture } from "../testing/mefixture";

// A seat that may book, so the menu carries its booking link.
const BOOKER = { allow: { activity: ["read", "create"] } } as const;

import { en as englishCopy } from "../i18n/en";
import { AccountMenu } from "./account";

// The bundle's releases are compiled in, and every local build leaves them
// empty. Getters over a hoisted value let each case set the releases it needs.
const bundle = vi.hoisted(() => ({ release: "", core: "", revision: "" }));
vi.mock("../lib/release", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../lib/release")>();
  return {
    ...actual,
    get SPA_RELEASE() {
      return bundle.release;
    },
    get CORE_RELEASE() {
      return bundle.core;
    },
    get BUILD_REVISION() {
      return bundle.revision;
    },
  };
});

// A pressed link would open a tab. The menu's own handlers still run, since
// they listen on the bubble path.
const stayHere = (event: MouseEvent) => event.preventDefault();

beforeEach(() => {
  document.addEventListener("click", stayHere, { capture: true });
});

afterEach(() => {
  cleanup();
  document.removeEventListener("click", stayHere, { capture: true });
  bundle.release = "";
  bundle.core = "";
  bundle.revision = "";
});

function renderMenu() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture(BOOKER));
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AccountMenu />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const trigger = () => screen.getByRole("button", { name: "TU Account" });
const row = (name: string | RegExp) => screen.getByRole("menuitem", { name });
const helpMenu = () =>
  screen.getByRole("menu", { name: englishCopy["shell.helpMenu"] });
const helpItem = (name: string | RegExp) =>
  within(helpMenu()).getByRole("menuitem", { name });

/** The account trigger, then Down to Help and feedback, then Right. */
async function openHelpByKeyboard(user: UserEvent) {
  trigger().focus();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu", { name: "Account" });
  for (let at = 0; at < 4; at++) {
    await user.keyboard("{ArrowDown}");
  }
  expect(document.activeElement).toBe(row(englishCopy["shell.helpMenu"]));
  await user.keyboard("{ArrowRight}");
}

async function openHelp(user: UserEvent) {
  await user.click(trigger());
  await screen.findByRole("menu", { name: "Account" });
  await user.click(row("Help and feedback"));
}

describe("Help and feedback", () => {
  it("opens from the keyboard on its first item, and walks its items", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);

    expect(row("Help and feedback")).toHaveAttribute("aria-expanded", "true");
    expect(row("Help and feedback")).toHaveAttribute(
      "aria-controls",
      helpMenu().id,
    );
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));

    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(helpItem(/^Request a feature/));
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(helpItem(englishCopy["about.title"]));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));
    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(helpItem(englishCopy["about.title"]));
    await user.keyboard("{Home}");
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));
    // The walk stayed inside: the account menu's own tabstop did not move.
    expect(row("Help and feedback")).toHaveAttribute("tabindex", "0");
  });

  it("goes back one layer at a time, with Left and with Escape", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);

    await user.keyboard("{ArrowLeft}");
    expect(
      screen.queryByRole("menu", { name: englishCopy["shell.helpMenu"] }),
    ).toBeNull();
    expect(document.activeElement).toBe(row(englishCopy["shell.helpMenu"]));

    await user.keyboard("{Enter}");
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));

    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("menu", { name: englishCopy["shell.helpMenu"] }),
    ).toBeNull();
    expect(document.activeElement).toBe(row(englishCopy["shell.helpMenu"]));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  // Help opens a menu. Theme opens radios, which no `aria-haspopup` value names.
  it("says the help row opens a menu, and the theme row nothing", async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(trigger());
    await screen.findByRole("menu", { name: "Account" });
    expect(row("Help and feedback")).toHaveAttribute("aria-haspopup", "menu");
    expect(row("Theme")).not.toHaveAttribute("aria-haspopup");
  });

  it("closes both layers when Tab leaves", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);

    await user.keyboard("{Tab}");
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens each place off the app in a new tab, and says so", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);

    const expected = [
      [
        "Report a problem",
        "https://github.com/margince/margince/issues/new?template=bug_report.yml",
      ],
      [
        "Request a feature",
        "https://github.com/margince/margince/issues/new?template=capability_gap.yml",
      ],
      ["Help and resources", "https://margince.com/resources/"],
      ["Release notes", "https://github.com/margince/margince/releases"],
    ];
    for (const [label, href] of expected) {
      const link = helpItem(`${label} (Opens in a new tab)`);
      expect(link).toHaveAttribute("href", href);
      expect(link).toHaveAttribute("target", "_blank");
      expect(link).toHaveAttribute("rel", "noopener noreferrer");
      expect(link.querySelector("svg[aria-hidden]")).not.toBeNull();
    }
  });

  it.each([
    ["v1.4.0", "", "v1.4.0"],
    ["v0.4.1", "v0.0.5", "v0.0.5"],
    ["dev", "v1.0.0+build/1", "v1.0.0%2Bbuild%2F1"],
  ])(
    "names the release on a stamped build (instance %j, core %j)",
    async (release, core, commit) => {
      bundle.release = release;
      bundle.core = core;
      const user = userEvent.setup();
      renderMenu();
      await openHelp(user);
      expect(helpItem(/^Report a problem/)).toHaveAttribute(
        "href",
        `https://github.com/margince/margince/issues/new?template=bug_report.yml&commit=${commit}`,
      );
    },
  );

  it.each([
    ["0123456789abcdef", "&commit=0123456"],
    ["dev", ""],
  ])(
    "names the revision of an unstamped build only when known (%j)",
    async (revision, query) => {
      bundle.revision = revision;
      const user = userEvent.setup();
      renderMenu();
      await openHelp(user);
      expect(helpItem(/^Report a problem/)).toHaveAttribute(
        "href",
        `https://github.com/margince/margince/issues/new?template=bug_report.yml${query}`,
      );
    },
  );

  it("closes the whole menu when a link is chosen", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);

    await user.click(helpItem(/^Help and resources/));
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens the shortcuts in a dialog, and hands focus back to the trigger", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}");
    expect(document.activeElement).toBe(
      helpItem(englishCopy["shell.help.shortcuts"]),
    );

    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", {
      name: englishCopy["shell.help.shortcuts"],
    });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(within(dialog).getByText("Open search")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", {
        name: englishCopy["shell.help.shortcuts"],
      }),
    ).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });
});

describe("the shortcuts dialog on ?", () => {
  // One dialog, whichever way it opens: Help and the key reach the same instance.
  it("opens the dialog Help opens, and hands focus back where the key was pressed", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    renderMenu();
    render(<button type="button">On the page</button>);
    const page = screen.getByRole("button", { name: "On the page" });
    page.focus();

    await user.keyboard("?");
    await screen.findByRole("dialog", {
      name: englishCopy["shell.help.shortcuts"],
    });
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    await user.keyboard("{Escape}");
    expect(document.activeElement).toBe(page);
  });
});

describe("About Margince at the foot of Help and feedback", () => {
  it("is the last item, with no separator before it, and says it opens a dialog", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    const about = helpItem("About Margince");
    expect(about).toHaveAttribute("aria-haspopup", "dialog");
    expect(within(helpMenu()).getAllByRole("menuitem").at(-1)).toBe(about);
    expect(about.previousElementSibling).toBe(
      helpItem(englishCopy["shell.help.shortcuts"]),
    );
    expect(helpMenu().querySelector("hr")).toBeNull();
  });

  it("lists the places off the app first, then the rows that stay in it", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(
      within(helpMenu())
        .getAllByRole("menuitem")
        .map((item) => item.textContent?.replace(" (Opens in a new tab)", "")),
    ).toEqual([
      englishCopy["shell.help.report"],
      englishCopy["shell.help.request"],
      englishCopy["shell.help.resources"],
      englishCopy["shell.help.releaseNotes"],
      englishCopy["shell.help.shortcuts"],
      englishCopy["about.title"],
    ]);
  });

  it("names no version in the menu", async () => {
    bundle.release = "v1.4.0";
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(within(helpMenu()).queryByText(/v1\.4\.0/)).toBeNull();
  });

  it("leaves the licenses to About", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(
      within(helpMenu()).queryByRole("menuitem", { name: /licenses/i }),
    ).toBeNull();
  });

  it("opens the About dialog from the keyboard on its version, and hands focus back to the trigger", async () => {
    bundle.release = "v1.4.0";
    bundle.revision = "0123456789abcdef0123456789abcdef01234567";
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(helpItem(englishCopy["about.title"]));

    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", {
      name: englishCopy["about.title"],
    });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(
      within(dialog).getByRole("button", { name: /^v1\.4\.0 · 0123456 / }),
    );

    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: englishCopy["about.title"] }),
    ).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  it("opens the About dialog on a click", async () => {
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    await user.click(helpItem("About Margince"));
    const dialog = await screen.findByRole("dialog", {
      name: englishCopy["about.title"],
    });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(
      within(dialog).getByRole("button", { name: /^Development build / }),
    ).toBeVisible();
  });
});
