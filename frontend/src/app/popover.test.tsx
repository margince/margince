/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { afterEach, expect, it } from "vitest";
import { usePopoverDismiss } from "./popover";

afterEach(cleanup);

function AccountMenu() {
  const [open, setOpen] = useState(true);
  const panel = useRef<HTMLOutputElement | null>(null);
  usePopoverDismiss(open, panel, () => setOpen(false));
  return <output ref={panel}>{open ? "open" : "closed"}</output>;
}

it("closes on Escape", async () => {
  render(<AccountMenu />);

  await userEvent.keyboard("{Escape}");

  expect(screen.getByRole("status")).toHaveTextContent("closed");
});

it("leaves an Escape a layer above already handled", async () => {
  const claim = (event: KeyboardEvent) => event.preventDefault();
  globalThis.addEventListener("keydown", claim, true);
  render(<AccountMenu />);

  await userEvent.keyboard("{Escape}");
  globalThis.removeEventListener("keydown", claim, true);

  expect(screen.getByRole("status")).toHaveTextContent("open");
});
