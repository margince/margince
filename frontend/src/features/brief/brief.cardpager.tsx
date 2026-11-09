// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import "./brief.cardpager.css";
import { Button } from "../../design-system/atoms";
import type { Drag } from "../../design-system/decisiondeck.stack";
import { swipeSide } from "../../design-system/swiperow";
import { useT } from "../../i18n";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";

export function CardPager({
  previous,
  next,
  title,
  children,
}: Readonly<{
  previous?: () => void;
  next?: () => void;
  title: string;
  children: ReactNode;
}>) {
  const t = useT();
  const [drag, setDrag] = useState<Drag | null>(null);
  const [hint, setHint] = useState(
    () => readStored(STORAGE_KEYS.homeSwipeSeen) !== "yes",
  );
  const move = (action: (() => void) | undefined) => {
    if (!action) return;
    action();
    setHint(false);
    writeStored(STORAGE_KEYS.homeSwipeSeen, "yes");
  };
  const resisted =
    drag && ((drag.dx > 0 && !previous) || (drag.dx < 0 && !next));
  const offset = drag ? drag.dx / (resisted ? 5 : 1) : 0;
  return (
    <section className="brief-cardpager" aria-label={t("brief.pager.label")}>
      <div
        className={
          next
            ? "brief-cardpager-face brief-cardpager-stacked"
            : "brief-cardpager-face"
        }
        data-dragging={drag !== null}
        style={{ transform: `translateX(${offset}px)` }}
        onPointerDown={(event) => {
          const target = event.target instanceof Element ? event.target : null;
          if (
            event.button !== 0 ||
            !event.isPrimary ||
            target?.closest("button,a,input,textarea,select,[role='button']")
          )
            return;
          setDrag({
            pointerId: event.pointerId,
            startX: event.clientX,
            startY: event.clientY,
            dx: 0,
            dy: 0,
          });
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (drag?.pointerId !== event.pointerId) return;
          const dx = event.clientX - drag.startX;
          const dy = event.clientY - drag.startY;
          setDrag({ ...drag, dx: Math.abs(dx) > Math.abs(dy) ? dx : 0, dy });
        }}
        onPointerUp={(event) => {
          if (drag?.pointerId !== event.pointerId) return;
          const side = swipeSide(
            event.clientX - drag.startX,
            event.clientY - drag.startY,
          );
          setDrag(null);
          if (side) move(side === "start" ? next : previous);
        }}
        onPointerCancel={() => setDrag(null)}
      >
        {children}
      </div>
      <fieldset
        className="form-actions"
        aria-label={t("brief.pager.label")}
        onKeyDown={(event) => {
          if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
          event.preventDefault();
          move(event.key === "ArrowRight" ? next : previous);
        }}
      >
        <Button disabled={!previous} onClick={() => move(previous)}>
          {t("brief.pager.previous")}
        </Button>
        <Button disabled={!next} onClick={() => move(next)}>
          {t("brief.pager.next")}
        </Button>
      </fieldset>
      {hint && <p className="t-caption">{t("brief.pager.hint")}</p>}
      <span role="status" className="sr-only">
        {title}
      </span>
    </section>
  );
}
