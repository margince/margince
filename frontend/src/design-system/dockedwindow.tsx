// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronUp, Maximize2, Minimize2, Minus, X } from "lucide-react";
import {
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { useFocusHandoff } from "./focushandoff";
import { Heading } from "./heading";
import { IconAction } from "./iconaction";
import "./dockedwindow.css";

/** How the window sits: at its own size, grown, or folded to its title bar. */
export type DockState = "open" | "max" | "min";

type Size = Readonly<{ width: number; height: number }>;

// Smaller than this and a To line, a subject and three lines of text no
// longer fit; the glass bounds the other end.
const SMALLEST: Size = { width: 380, height: 320 };
const GUTTER = 48;
// Where the window becomes a sheet (dockedwindow.css); a sheet does not slide.
const SHEET = "(max-width: 720px)";

function clamp(value: number, low: number, high: number) {
  return Math.min(Math.max(value, low), Math.max(low, high));
}

/**
 * Work that stays open beside the page: a window docked to the bottom end
 * corner. Nothing behind it is dimmed or made inert, and focus is free to
 * leave, so the reader can read the record while writing.
 *
 * It folds to its title bar (Escape too), grows, or sizes by its top and
 * start edges. Its title bar slides it along the bottom edge. Folding
 * keeps the content mounted. On a phone it is a sheet.
 */
export function DockedWindow({
  title,
  aside,
  state,
  onStateChange,
  onClose,
  closeDisabled,
  initialFocusTo,
  openedFrom,
  children,
}: Readonly<{
  /** Names the window and its folded bar; a heading, not a dialog title row. */
  title: string;
  /**
   * What the title names, beside it: the record this window's work belongs to.
   *
   * In the bar rather than in the content. The content is a writing surface
   * with a floor under it, and a row above the fields comes out of that floor.
   *
   * The folded bar does not carry it: that bar is one button, and a link inside
   * a button is markup no reader can use.
   */
  aside?: ReactNode;
  state: DockState;
  onStateChange: (next: DockState) => void;
  onClose: () => void;
  /** Holds the close while a write the window started is still out. */
  closeDisabled?: boolean;
  /** The control focus lands on when the window first opens. */
  initialFocusTo?: () => HTMLElement | null;
  /** The control that opened the window, which gets focus back on close. */
  openedFrom?: HTMLElement;
  children: ReactNode;
}>) {
  const t = useT();
  const titleId = useId();
  const box = useRef<HTMLElement | null>(null);
  const [size, setSize] = useState<Size | null>(null);
  // Where a drag started, held as state. The handlers that read it are made
  // during render, where the compiler refuses a ref read.
  const [drag, setDrag] = useState<{
    x: number;
    y: number;
    from: Size;
    edges: Readonly<{ start: boolean; top: boolean }>;
  } | null>(null);
  // How far the window has been slid from its corner along the bottom edge,
  // and where a slide started. Pointer only, like the sizing edges: where it
  // sits is the reader's preference, and every control works in the corner.
  const [shift, setShift] = useState(0);
  const [slide, setSlide] = useState<{
    x: number;
    from: number;
    room: number;
    toward: 1 | -1;
  } | null>(null);
  const [sheet, setSheet] = useState(
    () => globalThis.matchMedia?.(SHEET).matches ?? false,
  );
  const focusFirst = useRef(initialFocusTo);
  // The pressed control, else where focus was: Safari does not focus a clicked
  // button. Closing hands focus back there instead of to the page body.
  const opener = useRef(openedFrom ?? null);
  useEffect(() => {
    const before = document.activeElement;
    if (
      !opener.current &&
      before instanceof HTMLElement &&
      before !== document.body
    ) {
      opener.current = before;
    }
    (focusFirst.current?.() ?? box.current)?.focus();
  }, []);
  useFocusHandoff(box, () =>
    opener.current?.isConnected ? opener.current : null,
  );
  // A narrower glass pulls a slid window back into view. The breakpoint that
  // makes it a sheet is read again too, so neither waits for another render.
  useEffect(() => {
    const fit = () => {
      setSheet(globalThis.matchMedia?.(SHEET).matches ?? false);
      const node = box.current;
      if (!node) return;
      const end = node.getBoundingClientRect();
      const rtl = getComputedStyle(node).direction === "rtl";
      const spare = rtl ? window.innerWidth - end.right : end.left;
      setShift((was) => clamp(was, 0, was + spare));
    };
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, []);

  const grab =
    (edges: Readonly<{ start: boolean; top: boolean }>) =>
    (event: PointerEvent<HTMLDivElement>) => {
      const node = box.current;
      if (!node) return;
      event.preventDefault();
      event.currentTarget.setPointerCapture(event.pointerId);
      setDrag({
        x: event.clientX,
        y: event.clientY,
        from: { width: node.offsetWidth, height: node.offsetHeight },
        edges,
      });
    };
  const move = (event: PointerEvent<HTMLDivElement>) => {
    const from = drag;
    if (!from) return;
    // The window grows toward the page: dragging the start edge away from
    // the corner widens it, dragging the top edge up makes it taller.
    const dx = from.edges.start ? from.x - event.clientX : 0;
    const dy = from.edges.top ? from.y - event.clientY : 0;
    setSize({
      width: clamp(
        from.from.width + dx,
        SMALLEST.width,
        window.innerWidth - GUTTER,
      ),
      height: clamp(
        from.from.height + dy,
        SMALLEST.height,
        window.innerHeight - GUTTER,
      ),
    });
  };
  const release = () => setDrag(null);
  const grip = (edges: Readonly<{ start: boolean; top: boolean }>) => ({
    onPointerDown: grab(edges),
    onPointerMove: move,
    onPointerUp: release,
    onPointerCancel: release,
  });
  const lift = (event: PointerEvent<HTMLDivElement>) => {
    const node = box.current;
    // The bar's own controls keep their press; only its open field slides.
    // Links as well as buttons: capturing the pointer retargets the click at
    // the bar, and a reference beside the title could not be followed.
    if (
      !node ||
      sheet ||
      (event.target instanceof Element &&
        event.target.closest("button, a[href]"))
    ) {
      return;
    }
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    const end = node.getBoundingClientRect();
    const rtl = getComputedStyle(node).direction === "rtl";
    setSlide({
      x: event.clientX,
      from: shift,
      // The room between the window and the far edge of the glass.
      room: shift + (rtl ? window.innerWidth - end.right : end.left),
      toward: rtl ? -1 : 1,
    });
  };
  const slideTo = (event: PointerEvent<HTMLDivElement>) => {
    if (!slide) return;
    const moved = (slide.x - event.clientX) * slide.toward;
    setShift(clamp(slide.from + moved, 0, slide.room));
  };
  const drop = () => setSlide(null);
  const fold = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Escape" || event.defaultPrevented) return;
    event.preventDefault();
    onStateChange("min");
  };
  const grown = state === "max";
  // Portalled to the end of the body, after the dialog it was opened from:
  // at the overlays' level, document order is what draws it above that dialog.
  return createPortal(
    <>
      <section
        ref={box}
        role="dialog"
        aria-modal="false"
        aria-labelledby={titleId}
        className={grown ? "docked is-grown" : "docked"}
        hidden={state === "min"}
        tabIndex={-1}
        onKeyDown={fold}
        style={{
          ...(size && !grown
            ? { inlineSize: `${size.width}px`, blockSize: `${size.height}px` }
            : {}),
          ...(shift && !sheet ? { marginInlineEnd: `${shift}px` } : {}),
        }}
      >
        {/* Pointer-only: the grow control is the keyboard's way to size it. */}
        <div
          className="docked-grip docked-grip-top"
          aria-hidden="true"
          {...grip({ start: false, top: true })}
        />
        <div
          className="docked-grip docked-grip-start"
          aria-hidden="true"
          {...grip({ start: true, top: false })}
        />
        <div
          className="docked-grip docked-grip-corner"
          aria-hidden="true"
          {...grip({ start: true, top: true })}
        />
        <div
          className={slide ? "docked-head is-sliding" : "docked-head"}
          onPointerDown={lift}
          onPointerMove={slideTo}
          onPointerUp={drop}
          onPointerCancel={drop}
        >
          <Heading as="h2" size="xsmall" id={titleId} className="docked-title">
            {title}
          </Heading>
          {aside}
          <IconAction
            label={t("common.minimize")}
            icon={<Minus aria-hidden="true" />}
            onClick={() => onStateChange("min")}
          />
          <IconAction
            label={t(grown ? "common.collapse" : "common.expand")}
            icon={
              grown ? (
                <Minimize2 aria-hidden="true" />
              ) : (
                <Maximize2 aria-hidden="true" />
              )
            }
            onClick={() => {
              setSize(null);
              onStateChange(grown ? "open" : "max");
            }}
          />
          <IconAction
            label={t("common.close")}
            icon={<X aria-hidden="true" />}
            disabled={closeDisabled}
            onClick={onClose}
          />
        </div>
        <div className="docked-body">{children}</div>
      </section>
      {state === "min" && (
        <button
          type="button"
          className="docked-bar"
          aria-label={t("common.restoreNamed", { name: title })}
          onClick={() => onStateChange("open")}
        >
          <span className="docked-bar-title">{title}</span>
          <ChevronUp aria-hidden="true" />
        </button>
      )}
    </>,
    document.body,
  );
}
