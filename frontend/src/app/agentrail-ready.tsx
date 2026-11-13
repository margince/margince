// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useRef } from "react";
import { usePrefersReducedMotion } from "../design-system/motion";
import { type PeekContent, usePeek } from "../design-system/peek";
import { taskLine, taskName } from "../features/brief/brief.taskcard";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useMe } from "../lib/session";
import { useViewerZone } from "../lib/viewerzone";

/** The next row in one card, on hover or focus of the orb, while the panel is shut. */
export function useReadyPeek(
  rows: readonly WorklistItem[] | undefined,
  shut: boolean,
  onOpen: () => void,
  anchor: RefObject<HTMLButtonElement | null>,
) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const viewer = useMe(false).data?.user;
  const first = rows?.[0];
  const content: PeekContent | undefined =
    first && rows && shut
      ? {
          head: t("agent.ready.peekHead"),
          items: [
            {
              key: `${first.source}-${first.id}`,
              title: taskName(first, viewer, t, locale),
              sub: taskLine(first, t, locale, zone) ?? undefined,
              onOpen,
            },
          ],
          foot: plural("agent.ready.peekFoot", rows.length, {
            count: formatNumber(rows.length, locale),
          }),
        }
      : undefined;
  return usePeek({ content, focusable: true, anchor });
}

/** How long after a press a shrinking count is still that press's answer. */
const PRESS_ANSWERED_MS = 8_000;

type Press = Readonly<{ rect: DOMRect; at: number }>;

/**
 * When the day loses a row soon after a press anywhere, a spark flies from that
 * press into the orb. The count is the one signal every verb shares, so no verb
 * has to announce itself. A drop with no recent press (a refetch) flies nothing.
 */
export function useSettledFlight(
  count: number | undefined,
  orb: RefObject<HTMLElement | null>,
) {
  const reduced = usePrefersReducedMotion();
  const press = useRef<Press | null>(null);
  const was = useRef(count);
  useEffect(() => {
    const remember = (event: PointerEvent) => {
      if (event.target instanceof Element) {
        press.current = {
          rect: event.target.getBoundingClientRect(),
          at: Date.now(),
        };
      }
    };
    document.addEventListener("pointerdown", remember, true);
    return () => document.removeEventListener("pointerdown", remember, true);
  }, []);
  useEffect(() => {
    const before = was.current;
    was.current = count;
    const from = press.current;
    const to = orb.current;
    if (
      reduced ||
      before === undefined ||
      count === undefined ||
      count >= before ||
      from === null ||
      to === null ||
      Date.now() - from.at > PRESS_ANSWERED_MS
    ) {
      return;
    }
    press.current = null;
    fly(from.rect, to.getBoundingClientRect());
  }, [count, orb, reduced]);
}

function fly(from: DOMRect, to: DOMRect) {
  const mark = document.createElement("span");
  mark.className = "arflight";
  mark.setAttribute("aria-hidden", "true");
  const x = from.left + from.width / 2;
  const y = from.top + from.height / 2;
  mark.style.left = `${x}px`;
  mark.style.top = `${y}px`;
  mark.style.setProperty("--arFlyX", `${to.left + to.width / 2 - x}px`);
  mark.style.setProperty("--arFlyY", `${to.top + to.height / 2 - y}px`);
  mark.addEventListener("animationend", () => mark.remove());
  document.body.appendChild(mark);
}
