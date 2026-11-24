/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type Toast, ToastProvider, ToastRegion, useToast } from "./toast";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function mount(): () => Toast {
  let held: Toast | null = null;
  function Capture() {
    held = useToast();
    return null;
  }
  render(
    <LocaleProvider initial="en">
      <ToastProvider>
        <Capture />
        <ToastRegion />
      </ToastProvider>
    </LocaleProvider>,
  );
  return () => {
    if (held === null) {
      throw new Error("the toast controls were never rendered");
    }
    return held;
  };
}

// A select opened under the first message stands above the region until the
// region is shown again, so a replacement could sit beneath it.
it("raises the region above later layers when a message replaces another", () => {
  const toast = mount();
  act(() => {
    toast().show("First done", { sticky: true });
  });
  const hide = vi.spyOn(HTMLElement.prototype, "hidePopover");
  const show = vi.spyOn(HTMLElement.prototype, "showPopover");
  act(() => {
    toast().show("Second done", { sticky: true });
  });
  expect(screen.getByRole("status")).toHaveTextContent("Second done");
  const region = document.querySelector("[popover]");
  expect(hide.mock.contexts).toEqual([region]);
  expect(show.mock.contexts).toEqual([region]);
});
