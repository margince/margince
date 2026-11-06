// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, waitFor, within } from "storybook/test";
import { en } from "../i18n/en";

// The width at which brief.css stacks the head and drops the dial under it.
const HEAD_STACK_MAX_PX = 640;

function drawn(rect: DOMRect | null | undefined, what: string): DOMRect {
  if (!rect) throw new Error(`the head drew no ${what}`);
  return rect;
}

/** The view dial ends the head's last line, or stands under the head on a phone. */
export async function expectDialPlacement(canvasElement: HTMLElement) {
  const dial = await within(canvasElement).findByRole("group", {
    name: en["brief.view.label"],
  });
  const at = dial.getBoundingClientRect();
  const glance = dial.closest(".glance");
  const head = drawn(
    glance?.querySelector(".glance-head")?.getBoundingClientRect(),
    "head",
  );
  if (globalThis.matchMedia(`(max-width: ${HEAD_STACK_MAX_PX}px)`).matches) {
    await expect(at.top).toBeGreaterThanOrEqual(head.bottom - 0.5);
    return;
  }
  const end = drawn(glance?.getBoundingClientRect(), "glance").right;
  await expect(Math.abs(at.right - end)).toBeLessThan(1);
  await expect(at.left).toBeGreaterThanOrEqual(head.right - 0.5);
  await expect(at.bottom).toBeLessThanOrEqual(head.bottom + 0.5);
}

/** The weekly report runs the whole inner width of its padded pane. */
export async function expectReportFillsPane(canvasElement: HTMLElement) {
  const { report, pane } = await waitFor(() => {
    const found = canvasElement.querySelector(
      ".page-pane.brief-weekly-page > .brief-report",
    );
    if (!found?.parentElement) {
      throw new Error("the weekly drew no report on a pane");
    }
    return { report: found, pane: found.parentElement };
  });
  const style = getComputedStyle(pane);
  const inset =
    Number.parseFloat(style.paddingLeft) +
    Number.parseFloat(style.paddingRight);
  await expect(inset).toBeGreaterThan(0);
  await expect(
    Math.abs(report.getBoundingClientRect().width - (pane.clientWidth - inset)),
  ).toBeLessThan(1);
}
