/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { createPortal } from "react-dom";
import { afterEach, expect, it } from "vitest";
import { Button } from "./atoms";
import { OverflowMenu } from "./overflowmenu";

afterEach(cleanup);

// An item that opens its own layer, portalled out of the panel as a dialog is.
// Its key events still bubble through the menu in React's tree.
function OpensALayer() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button aria-expanded={open} onClick={() => setOpen(true)}>
        Share
      </Button>
      {open &&
        createPortal(
          <div role="dialog" aria-label="Share">
            <button type="button">Copy link</button>
            <button type="button">Invite</button>
          </div>,
          document.body,
        )}
    </>
  );
}

function Menu() {
  return (
    <OverflowMenu label="More actions">
      <Button>Merge</Button>
      <OpensALayer />
    </OverflowMenu>
  );
}

it("walks its own items with Down and Up", async () => {
  const user = userEvent.setup();
  render(<Menu />);
  screen.getByRole("button", { name: "More actions" }).focus();

  await user.keyboard("{ArrowDown}");
  expect(screen.getByRole("button", { name: "Merge" })).toHaveFocus();
  await user.keyboard("{ArrowDown}");
  expect(screen.getByRole("button", { name: "Share" })).toHaveFocus();
  await user.keyboard("{ArrowUp}");
  expect(screen.getByRole("button", { name: "Merge" })).toHaveFocus();
});

it("leaves the arrows of a layer an item opened to that layer", async () => {
  const user = userEvent.setup();
  render(<Menu />);
  await user.click(screen.getByRole("button", { name: "More actions" }));
  await user.click(screen.getByRole("button", { name: "Share" }));
  const copy = screen.getByRole("button", { name: "Copy link" });
  copy.focus();

  await user.keyboard("{ArrowDown}");

  expect(copy).toHaveFocus();
});

it("consumes the arrow at either end, so the page does not scroll", async () => {
  const user = userEvent.setup();
  render(<Menu />);
  await user.click(screen.getByRole("button", { name: "More actions" }));
  const last = screen.getByRole("button", { name: "Share" });
  last.focus();

  expect(fireEvent.keyDown(last, { key: "ArrowDown" })).toBe(false);
  expect(last).toHaveFocus();
});
