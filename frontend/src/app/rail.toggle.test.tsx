// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../i18n/en";
import { render, stubPhoneViewport } from "../testing/shellharness";
import { WorkspaceRail } from "./shell";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const ignoreToggle = () => undefined;

// The panel's own width control stands at its foot, under the agent.
describe("the sidebar toggle at the rail's foot", () => {
  it("reads Collapse sidebar expanded and calls the handler on a press", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(<WorkspaceRail route={{ screen: "home" }} onToggle={onToggle} />);

    const toggle = screen.getByRole("button", {
      name: englishCopy["shell.collapse"],
    });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(toggle).toHaveTextContent(englishCopy["shell.collapse"]);
    await user.click(toggle);
    expect(onToggle).toHaveBeenCalledTimes(1);
  });

  // The name says what a press does next while `aria-expanded` reports the
  // state the panel is in; either alone passes against a control that never
  // changes.
  it("is named Expand sidebar collapsed, and its tip adds the keys", () => {
    render(
      <WorkspaceRail
        route={{ screen: "home" }}
        collapsed
        onToggle={ignoreToggle}
      />,
    );
    const toggle = screen.getByRole("button", {
      name: englishCopy["shell.expand"],
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(toggle).toHaveAttribute("aria-keyshortcuts", "Meta+B Control+B");

    act(() => toggle.focus());
    expect(screen.getByRole("tooltip").textContent).toMatch(
      /^Expand sidebar · (⌘|Ctrl) B$/,
    );
  });

  it("stands after the agent, as the panel's last row", () => {
    const { container } = render(
      <WorkspaceRail route={{ screen: "home" }} onToggle={ignoreToggle} />,
    );
    const foot = container.querySelector(".rail > :last-child");
    expect(foot).toHaveClass("railfoot");
    expect(foot?.previousElementSibling).toHaveClass("railagent");
  });

  // The handler is the only evidence the panel has that something can fold it.
  it("draws no control when it is handed no toggle", () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);
    expect(screen.queryByRole("button", { name: /sidebar$/ })).toBeNull();
  });

  // The phone bar is not a panel that folds.
  it("draws no control at phone width", () => {
    stubPhoneViewport();
    render(
      <WorkspaceRail route={{ screen: "home" }} onToggle={ignoreToggle} />,
    );
    expect(screen.queryByRole("button", { name: /sidebar$/ })).toBeNull();
  });
});

// A label keeps one line, so a translation too long for the open rail is cut.
describe("a row the open rail cuts", () => {
  it("shows its whole label as the tip", () => {
    vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(240);
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(192);
    render(<WorkspaceRail route={{ screen: "home" }} />);

    act(() =>
      screen.getByRole("link", { name: englishCopy["filters.title"] }).focus(),
    );
    expect(screen.getByRole("tooltip").textContent).toBe(
      englishCopy["filters.title"],
    );
  });

  it("keeps the go-to keys on a row that fits", () => {
    render(<WorkspaceRail route={{ screen: "home" }} />);

    act(() => screen.getByRole("link", { name: "Leads" }).focus());
    expect(screen.getByRole("tooltip").textContent).toBe("G then L");
  });
});
