// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import {
  type FocusEvent,
  type MouseEvent,
  type PointerEvent,
  useEffect,
  useRef,
} from "react";
import { useHoverIntent } from "../design-system/hoverintent";
import { opensElsewhere } from "../lib/linkclick";
import { routeHash } from "../lib/router";
import { type NavLevelEntry, type NavTrailLevel, navEntryHref } from "./nav";
import { navigationPrefetch, type PrefetchTarget } from "./navigation-prefetch";

const TARGETS: readonly PrefetchTarget[] = ["contacts", "deals"];

// Only a bare primary link to a warmable screen names its read; any other row,
// and the row already open, warms nothing.
function targetOf(
  level: NavTrailLevel,
  entry: NavLevelEntry,
  active: boolean,
): PrefetchTarget | undefined {
  if (active || level.path.length > 0) return undefined;
  const href = navEntryHref(level.path, entry);
  return TARGETS.find((screen) => routeHash({ screen }) === href);
}

/**
 * The rail row's navigation intent: a settled mouse pointer or a visible
 * keyboard focus warms the destination's first read, and leaving releases it.
 */
function useNavigationIntent(
  level: NavTrailLevel,
  entry: NavLevelEntry,
  active: boolean,
) {
  const client = useQueryClient();
  const target = targetOf(level, entry, active);
  // Whether the pointer that arrived may start speculative work: a touch, a
  // drag or a pressed button may not.
  const pointerMayWarm = useRef(false);
  useEffect(() => {
    if (!target) return;
    return () => navigationPrefetch(client).leave(target);
  }, [client, target]);
  return {
    onPointerEnter: (event: PointerEvent) => {
      pointerMayWarm.current =
        event.pointerType !== "touch" && event.buttons === 0;
    },
    onSettledPointer: () => {
      if (target && pointerMayWarm.current)
        navigationPrefetch(client).intend(target);
    },
    onFocus: (event: FocusEvent) => {
      if (target && event.currentTarget.matches(":focus-visible"))
        navigationPrefetch(client).intend(target);
    },
    onLeave: () => {
      if (target) navigationPrefetch(client).leave(target);
    },
    onClick: (event: MouseEvent) => {
      if (target && event.button === 0 && !opensElsewhere(event))
        navigationPrefetch(client).follow(target);
    },
  };
}

/**
 * A rail row's pointer, focus and click handlers: the row's tooltip and its
 * navigation intent, composed so neither replaces the other.
 */
export function useRowIntent({
  level,
  entry,
  active,
  raiseTip,
  closeTip,
  select,
}: Readonly<{
  level: NavTrailLevel;
  entry: NavLevelEntry;
  active: boolean;
  raiseTip: (by: "pointer" | "focus") => void;
  closeTip: () => void;
  select: () => void;
}>) {
  const intent = useNavigationIntent(level, entry, active);
  // The rail's tips wait for the pointer to settle like every other popover.
  // A rail crossed on the way to the work column fired one tip per row it
  // passed, which read as the rail flickering rather than as a page answering.
  const hover = useHoverIntent(() => {
    raiseTip("pointer");
    intent.onSettledPointer();
  }, closeTip);
  return {
    onPointerEnter: (event: PointerEvent) => {
      intent.onPointerEnter(event);
      hover.onPointerEnter();
    },
    onPointerLeave: () => {
      hover.onPointerLeave();
      intent.onLeave();
    },
    onFocus: (event: FocusEvent) => {
      raiseTip("focus");
      intent.onFocus(event);
    },
    onBlur: () => {
      closeTip();
      intent.onLeave();
    },
    onClick: (event: MouseEvent) => {
      intent.onClick(event);
      select();
    },
  };
}
