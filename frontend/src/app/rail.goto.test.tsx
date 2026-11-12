// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { goToBindingFor } from "../lib/keymap";
import { routeHash } from "../lib/router";
import {
  setSingleKeyShortcutsOn,
  useShortcutDispatcher,
} from "../lib/shortcuts";
import { meFixture } from "../testing/mefixture";
import { newClient, render, renderWith } from "../testing/shellharness";
import { NAV } from "./nav";
import { WorkspaceRail } from "./shell";

afterEach(() => {
  cleanup();
  act(() => setSingleKeyShortcutsOn(true));
  window.location.hash = "";
});

describe("the destinations' go-to keys", () => {
  // The rail and the dialog name a destination with one word.
  it("bind every destination, in the rail's own words", () => {
    for (const item of NAV) {
      const bound = goToBindingFor(item.screen);
      expect(bound?.labelKey, item.screen).toBe(item.labelKey);
    }
  });

  // ARIA separates alternatives with a space, so "g c" would claim C alone goes there.
  it("put no aria-keyshortcuts on a row, which has no spelling for a sequence", () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);
    expect(screen.getByRole("link", { name: "Contacts" })).not.toHaveAttribute(
      "aria-keyshortcuts",
    );
  });

  it("show on an expanded row's tip, which the label already names", async () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);
    act(() => screen.getByRole("link", { name: "Leads" }).focus());
    expect((await screen.findByRole("tooltip")).textContent).toBe("G then L");
  });

  it("take the reader there on G, then the letter", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    render(<WorkspaceRail route={{ screen: "home" }} />);

    await user.keyboard("gp");
    expect(window.location.hash).toBe(routeHash({ screen: "projects" }));
  });

  // G is both the lead and the letter for Sales goals.
  it("take the reader to Sales goals on G twice", async () => {
    const user = userEvent.setup();
    renderHook(() => useShortcutDispatcher());
    render(<WorkspaceRail route={{ screen: "home" }} />);

    await user.keyboard("gg");
    expect(window.location.hash).toBe(routeHash({ screen: "goals" }));
  });

  it("leave a collapsed tip with the label alone while single keys are off", async () => {
    act(() => setSingleKeyShortcutsOn(false));
    render(<WorkspaceRail route={{ screen: "home" }} collapsed />);
    const deals = screen.getByRole("link", { name: "Deals" });
    act(() => deals.focus());
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      en["nav.deals"],
    );
  });

  it("raise no tip on an expanded row while single keys are off", () => {
    act(() => setSingleKeyShortcutsOn(false));
    render(<WorkspaceRail route={{ screen: "home" }} />);
    act(() => screen.getByRole("link", { name: "Deals" }).focus());
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});

describe("a destination the installation turned off", () => {
  it("takes nobody there on G, then its letter, and its row is gone", async () => {
    const user = userEvent.setup();
    const client = newClient();
    client.setQueryData(
      ["me"],
      meFixture({ settingsAvailability: { disabled_features: ["contacts"] } }),
    );
    window.location.hash = "#/home";
    renderHook(() => useShortcutDispatcher());
    renderWith(client, <WorkspaceRail route={{ screen: "home" }} />);
    expect(screen.queryByRole("link", { name: "Contacts" })).toBeNull();

    await user.keyboard("gc");
    expect(window.location.hash).toBe("#/home");
    await user.keyboard("gd");
    expect(window.location.hash).toBe(routeHash({ screen: "deals" }));
  });
});
