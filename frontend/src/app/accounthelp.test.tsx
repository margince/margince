// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { AccountMenu } from "./account";

// The bundle's releases are compiled in, and every local build leaves them
// empty. Getters over a hoisted value let each case set the releases it needs.
const bundle = vi.hoisted(() => ({ release: "", core: "" }));
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
});

function renderMenu() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture());
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AccountMenu />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const trigger = () => screen.getByRole("button", { name: "Account" });
const row = (name: string | RegExp) => screen.getByRole("menuitem", { name });
const helpMenu = () => screen.getByRole("menu", { name: "Help and feedback" });
const helpItem = (name: string | RegExp) =>
  within(helpMenu()).getByRole("menuitem", { name });

/** The account trigger, then Down to Help and feedback, then Right. */
async function openHelpByKeyboard(user: UserEvent) {
  trigger().focus();
  await user.keyboard("{Enter}");
  for (let at = 0; at < 4; at++) {
    await user.keyboard("{ArrowDown}");
  }
  expect(document.activeElement).toBe(row("Help and feedback"));
  await user.keyboard("{ArrowRight}");
}

async function openHelp(user: UserEvent) {
  await user.click(trigger());
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
    expect(document.activeElement).toBe(helpItem(/^Release notes/));
    await user.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));
    await user.keyboard("{ArrowUp}");
    expect(document.activeElement).toBe(helpItem(/^Release notes/));
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
      screen.queryByRole("menu", { name: "Help and feedback" }),
    ).toBeNull();
    expect(document.activeElement).toBe(row("Help and feedback"));

    await user.keyboard("{Enter}");
    expect(document.activeElement).toBe(helpItem(/^Report a problem/));

    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("menu", { name: "Help and feedback" }),
    ).toBeNull();
    expect(document.activeElement).toBe(row("Help and feedback"));

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });

  // Help opens a menu. Theme opens radios, which no `aria-haspopup` value names.
  it("says the help row opens a menu, and the theme row nothing", async () => {
    const user = userEvent.setup();
    renderMenu();
    await user.click(trigger());
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
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}");
    expect(document.activeElement).toBe(helpItem("Keyboard shortcuts"));

    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", {
      name: "Keyboard shortcuts",
    });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(within(dialog).getByText("Open search")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: "Keyboard shortcuts" }),
    ).toBeNull();
    expect(document.activeElement).toBe(trigger());
  });
});

describe("the build at the foot of Help and feedback", () => {
  it.each(["", "dev"])(
    "says Development build when nothing is stamped (%j)",
    async (release) => {
      bundle.release = release;
      const user = userEvent.setup();
      renderMenu();
      await openHelp(user);
      const build = within(helpMenu()).getByText("Development build");
      expect(build.closest("[role=menuitem]")).toBeNull();
      expect(screen.queryByText(/^Version/)).toBeNull();
    },
  );

  // A statement, not a row. The keyboard walk ends on the last link above it.
  it("names a stamped release, outside the menu's items", async () => {
    bundle.release = "v1.4.0";
    const user = userEvent.setup();
    renderMenu();
    await openHelpByKeyboard(user);

    const version = within(helpMenu()).getByText("Version v1.4.0");
    expect(version.closest("[role=menuitem]")).toBeNull();
    expect(
      within(helpMenu()).getAllByRole("menuitem").at(-1)?.textContent,
    ).toMatch(/^Release notes/);
    await user.keyboard("{End}");
    expect(document.activeElement).toBe(helpItem(/^Release notes/));
  });

  it("names the instance and the core release on an instance build", async () => {
    bundle.release = "v0.4.1";
    bundle.core = "v0.0.5";
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(within(helpMenu()).getByText("Instance v0.4.1")).toBeInTheDocument();
    expect(within(helpMenu()).getByText("Core v0.0.5")).toBeInTheDocument();
    expect(screen.queryByText(/^Version/)).toBeNull();
  });

  // Equal values are still two facts about two repositories.
  it("keeps both lines when the instance and core releases are equal", async () => {
    bundle.release = "v0.4.1";
    bundle.core = "v0.4.1";
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(screen.getByText("Instance v0.4.1")).toBeInTheDocument();
    expect(screen.getByText("Core v0.4.1")).toBeInTheDocument();
  });

  it.each(["", "dev"])(
    "an instance build with an unstamped instance release (%j) names only the core",
    async (release) => {
      bundle.release = release;
      bundle.core = "v0.0.5";
      const user = userEvent.setup();
      renderMenu();
      await openHelp(user);
      expect(screen.getByText("Core v0.0.5")).toBeInTheDocument();
      expect(screen.queryByText(/^Instance/)).toBeNull();
      expect(screen.queryByText("Development build")).toBeNull();
    },
  );

  // "dev" is what a plain docker build stamps, so it is no core release.
  it('treats a core release of "dev" as a core-only build', async () => {
    bundle.release = "v1.4.0";
    bundle.core = "dev";
    const user = userEvent.setup();
    renderMenu();
    await openHelp(user);
    expect(screen.getByText("Version v1.4.0")).toBeInTheDocument();
    expect(screen.queryByText(/^Core/)).toBeNull();
  });

  it("is not in the account menu itself", async () => {
    bundle.release = "v1.4.0";
    const user = userEvent.setup();
    renderMenu();
    await user.click(trigger());
    expect(screen.queryByText("Version v1.4.0")).toBeNull();
  });
});
