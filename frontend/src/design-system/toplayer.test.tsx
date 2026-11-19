/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { useTopLayer } from "./toplayer";

function Layer({ open }: Readonly<{ open: boolean }>) {
  const ref = useRef<HTMLDivElement | null>(null);
  useTopLayer(ref, open);
  return open ? <div ref={ref} popover="manual" data-testid="layer" /> : null;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("shows the layer in the top layer once it opens, and not before", () => {
  const show = vi.spyOn(HTMLElement.prototype, "showPopover");
  const { rerender, getByTestId } = render(<Layer open={false} />);
  expect(show).not.toHaveBeenCalled();
  rerender(<Layer open />);
  expect(show).toHaveBeenCalledOnce();
  expect(show.mock.contexts[0]).toBe(getByTestId("layer"));
});

it("shows it once while it stays open across renders", () => {
  const show = vi.spyOn(HTMLElement.prototype, "showPopover");
  const { rerender } = render(<Layer open />);
  rerender(<Layer open />);
  expect(show).toHaveBeenCalledOnce();
});
