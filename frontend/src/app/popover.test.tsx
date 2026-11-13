/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, fireEvent, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePopoverDismiss } from "./popover";

function Panel({ onDismiss }: Readonly<{ onDismiss: () => void }>) {
  const panel = useRef<HTMLDivElement>(null);
  const [swapped, setSwapped] = useState(false);
  const [menu, setMenu] = useState(false);
  usePopoverDismiss(true, panel, onDismiss);
  return (
    <>
      <div ref={panel}>
        {/* A press that replaces its own button, as a card's Done does. */}
        {swapped ? (
          <span>Answered</span>
        ) : (
          <button type="button" onClick={() => setSwapped(true)}>
            Done
          </button>
        )}
        <button
          type="button"
          aria-expanded={menu}
          aria-controls="row-menu"
          onClick={() => setMenu(true)}
        >
          More
        </button>
      </div>
      {menu &&
        createPortal(
          <div id="row-menu">
            <button type="button">Snooze</button>
          </div>,
          document.body,
        )}
      <button type="button">Elsewhere</button>
    </>
  );
}

describe("usePopoverDismiss", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function open() {
    const onDismiss = vi.fn();
    render(<Panel onDismiss={onDismiss} />);
    // The outside-click listener joins a tick after the panel opens.
    act(() => vi.runAllTimers());
    return onDismiss;
  }

  it("stays open when a press inside removes the pressed button", () => {
    const onDismiss = open();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByText("Answered")).toBeTruthy();
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("stays open for a press in a menu the panel opened", () => {
    const onDismiss = open();
    fireEvent.click(screen.getByRole("button", { name: "More" }));
    fireEvent.click(screen.getByRole("button", { name: "Snooze" }));
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("closes on a press elsewhere", () => {
    const onDismiss = open();
    fireEvent.click(screen.getByRole("button", { name: "Elsewhere" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
