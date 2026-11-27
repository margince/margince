// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { opensElsewhere } from "../lib/linkclick";
import { parseHash } from "../lib/router";
import { contactTarget, navigationPrefetch } from "./navigation-prefetch";
import { opensReservedPage } from "./reservedpages";

// A pointer that rests this long on a link is heading for it; one passing over
// a list on its way elsewhere starts nothing.
export const HOVER_INTENT_MS = 80;

/** The contact whose page a link opens, unless that page is already open. */
export function contactOpenedBy(
  link: HTMLAnchorElement,
  current: string,
): string | undefined {
  const route = parseHash(link.hash);
  if (route.screen !== "contacts" || !route.id || opensReservedPage(route)) {
    return undefined;
  }
  const open = parseHash(current);
  return open.screen === "contacts" && open.id === route.id
    ? undefined
    : route.id;
}

function contactLinkOf(event: Event): string | undefined {
  const target = event.target;
  if (!(target instanceof Element)) return undefined;
  const link = target.closest("a[href]");
  return link instanceof HTMLAnchorElement
    ? contactOpenedBy(link, globalThis.location.hash)
    : undefined;
}

/**
 * Warms a contact page's reads from any link to it. A resting pointer, a press
 * or a focus starts them; leaving before the click lets them go.
 */
export function useRecordPrefetch(): void {
  const client = useQueryClient();
  useEffect(() => {
    const prefetch = navigationPrefetch(client);
    let pending: number | undefined;
    let hovered: string | undefined;
    const leaveHovered = () => {
      window.clearTimeout(pending);
      if (hovered) prefetch.leave(contactTarget(hovered));
      hovered = undefined;
    };
    const onOver = (event: PointerEvent) => {
      const id = contactLinkOf(event);
      if (id === hovered) return;
      leaveHovered();
      if (!id || event.pointerType === "touch" || event.buttons !== 0) return;
      hovered = id;
      pending = window.setTimeout(
        () => prefetch.intendContact(id),
        HOVER_INTENT_MS,
      );
    };
    // A pointer that leaves the page sends no further pointerover.
    const onOut = (event: PointerEvent) => {
      if (event.relatedTarget === null) leaveHovered();
    };
    const onPress = (event: PointerEvent) => {
      const id = contactLinkOf(event);
      if (!id || event.button !== 0) return;
      window.clearTimeout(pending);
      prefetch.intendContact(id);
    };
    const onFocus = (event: FocusEvent) => {
      const id = contactLinkOf(event);
      if (id && event.target instanceof Element) {
        if (event.target.matches(":focus-visible")) prefetch.intendContact(id);
      }
    };
    const onBlur = (event: FocusEvent) => {
      const id = contactLinkOf(event);
      if (id && id !== hovered) prefetch.leave(contactTarget(id));
    };
    const onClick = (event: MouseEvent) => {
      const id = contactLinkOf(event);
      if (id && event.button === 0 && !opensElsewhere(event))
        prefetch.follow(contactTarget(id));
    };
    document.addEventListener("pointerover", onOver);
    document.addEventListener("pointerout", onOut);
    document.addEventListener("pointerdown", onPress);
    document.addEventListener("focusin", onFocus);
    document.addEventListener("focusout", onBlur);
    // Capture runs before the link's own handler routes the click, so the
    // address read is still the page the reader clicked from.
    document.addEventListener("click", onClick, true);
    return () => {
      leaveHovered();
      document.removeEventListener("pointerover", onOver);
      document.removeEventListener("pointerout", onOut);
      document.removeEventListener("pointerdown", onPress);
      document.removeEventListener("focusin", onFocus);
      document.removeEventListener("focusout", onBlur);
      document.removeEventListener("click", onClick, true);
    };
  }, [client]);
}
