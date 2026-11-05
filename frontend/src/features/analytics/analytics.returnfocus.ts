// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect } from "react";
import { historyEntryId } from "../../lib/scrollmemory";

// The control a reader last used inside a surface, by history entry, so Back
// to that entry puts focus where they left. Memory per page load, as offsets
// in lib/scrollmemory.ts are: a reload has no place to return to.
const leftFrom = new Map<string, string>();

/** Forget every remembered control. For tests, which share one document. */
export function forgetReturnFocus(): void {
  leftFrom.clear();
}

/**
 * The control as a reader would name it: its kind, where it leads, what it
 * says and what it is described by. Element ids are minted per mount, so a
 * control coming back after Back has a new one and is found by this instead.
 */
function controlKey(element: Element): string {
  const describedBy = element.getAttribute("aria-describedby") ?? "";
  const description = describedBy
    .split(" ")
    .map((id) => element.ownerDocument.getElementById(id)?.textContent ?? "")
    .join(" ");
  return [
    element.tagName,
    element.getAttribute("href") ?? "",
    element.getAttribute("aria-label") ?? "",
    description,
    element.textContent ?? "",
  ].join("|");
}

const CONTROLS = "a[href], button";

function controlOf(target: EventTarget | null): Element | null {
  return target instanceof Element ? target.closest(CONTROLS) : null;
}

function findControl(surface: HTMLElement, key: string): HTMLElement | null {
  for (const control of surface.querySelectorAll<HTMLElement>(CONTROLS)) {
    if (controlKey(control) === key) return control;
  }
  return null;
}

// Focus the reader already moved somewhere is theirs, and is left alone.
function focusIsFree(document: Document): boolean {
  const active = document.activeElement;
  return active === null || active === document.body;
}

/**
 * Keep the control a reader leaves `surface` through, and focus it again
 * when they come back to the same history entry. The surface renders its
 * figures after it mounts, so the control is waited for as it arrives.
 */
export function useReturnFocus(surface: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const element = surface.current;
    if (!element) return;
    const entry = historyEntryId();
    let used: string | undefined;
    const note = (event: Event) => {
      const control = controlOf(event.target);
      if (control) used = controlKey(control);
    };
    element.addEventListener("focusin", note);
    element.addEventListener("click", note);
    const wanted = leftFrom.get(entry);
    leftFrom.delete(entry);
    const restore = (): boolean => {
      if (!wanted || !focusIsFree(element.ownerDocument)) return true;
      const control = findControl(element, wanted);
      control?.focus({ preventScroll: true });
      return control !== null;
    };
    const watcher = new MutationObserver(() => {
      if (restore()) watcher.disconnect();
    });
    if (!restore())
      watcher.observe(element, { childList: true, subtree: true });
    return () => {
      watcher.disconnect();
      element.removeEventListener("focusin", note);
      element.removeEventListener("click", note);
      if (used) leftFrom.set(entry, used);
    };
  }, [surface]);
}
