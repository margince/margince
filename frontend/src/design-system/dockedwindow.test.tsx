/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DockedWindow, type DockState } from "./dockedwindow";

afterEach(cleanup);

function Harness({ onClose = vi.fn() }: Readonly<{ onClose?: () => void }>) {
  const [state, setState] = useState<DockState>("open");
  return (
    <LocaleProvider initial="en">
      <button type="button">On the page</button>
      <DockedWindow
        title="Re: Budget"
        state={state}
        onStateChange={setState}
        onClose={onClose}
      >
        <input aria-label="Words" />
      </DockedWindow>
    </LocaleProvider>
  );
}

describe("a window docked beside the page", () => {
  it("is a dialog that leaves the page behind it in reach", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const window = screen.getByRole("dialog", { name: "Re: Budget" });
    expect(window.getAttribute("aria-modal")).toBe("false");
    await user.click(screen.getByRole("button", { name: "On the page" }));
    expect(document.activeElement?.textContent).toBe("On the page");
  });

  // Folding keeps what was typed: the content stays mounted under the bar.
  it("folds to its title bar and opens again with the words kept", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(screen.getByLabelText("Words"), "Half a sentence");
    await user.click(screen.getByRole("button", { name: "Minimize" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Open Re: Budget" }));
    expect((screen.getByLabelText("Words") as HTMLInputElement).value).toBe(
      "Half a sentence",
    );
  });

  it("folds on Escape, and closes only through its own control", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Harness onClose={onClose} />);
    fireEvent.keyDown(screen.getByLabelText("Words"), { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Open Re: Budget" }));
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("grows and shrinks back from its title bar", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Expand" }));
    expect(screen.getByRole("dialog").classList.contains("is-grown")).toBe(
      true,
    );
    await user.click(screen.getByRole("button", { name: "Collapse" }));
    expect(screen.getByRole("dialog").classList.contains("is-grown")).toBe(
      false,
    );
  });
});
