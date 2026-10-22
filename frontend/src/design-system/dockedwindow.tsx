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
import { useT } from "../i18n";
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

function clamp(value: number, low: number, high: number) {
  return Math.min(Math.max(value, low), Math.max(low, high));
}

/**
 * Work that stays open beside the page: a window docked to the bottom end
 * corner. Nothing behind it is dimmed or made inert, and focus is free to
 * leave, so the reader can read the record while writing.
 *
 * It folds to its title bar (Escape too), grows, or sizes by its top and
 * start edges. Folding keeps the content mounted. On a phone it is a sheet.
 */
export function DockedWindow({
  title,
  state,
  onStateChange,
  onClose,
  closeDisabled,
  initialFocusTo,
  children,
}: Readonly<{
  /** Names the window and its folded bar; a heading, not a dialog title row. */
  title: string;
  state: DockState;
  onStateChange: (next: DockState) => void;
  onClose: () => void;
  /** Holds the close while a write the window started is still out. */
  closeDisabled?: boolean;
  /** The control focus lands on when the window first opens. */
  initialFocusTo?: () => HTMLElement | null;
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
  const focusFirst = useRef(initialFocusTo);
  useEffect(() => {
    (focusFirst.current?.() ?? box.current)?.focus();
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
  const fold = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key !== "Escape" || event.defaultPrevented) return;
    event.preventDefault();
    onStateChange("min");
  };
  const grown = state === "max";
  return (
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
        style={
          size && !grown
            ? { inlineSize: `${size.width}px`, blockSize: `${size.height}px` }
            : undefined
        }
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
        <div className="docked-head">
          <Heading as="h2" size="xsmall" id={titleId} className="docked-title">
            {title}
          </Heading>
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
    </>
  );
}
