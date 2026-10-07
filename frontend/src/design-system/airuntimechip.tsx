// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import {
  type CSSProperties,
  type RefObject,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import type { components } from "../api/schema";
import { formatNumber, INTL_LOCALE } from "../format/format";
import type { Locale } from "../i18n";
import "./airuntimechip.css";
import { coveredByDialog } from "./dialogfocus";
import { useScrollRegion } from "./scrollregion";

type AiRunSummary = components["schemas"]["AiRunSummary"];

export type AiRuntimeLabels = Readonly<{
  configured: string;
  used: string;
  route: string;
  calls: string;
  tokens: string;
  latency: string;
  estimatedCost: string;
  partial: string;
  awaiting: string;
  unavailable: string;
  chip: string;
  answering: string;
  scope: string;
}>;

// Which model answered, how many calls, what it cost: the spend is always in
// view, and hover reveals the breakdown while a press pins it.
export function AiRuntimeChip({
  runtime,
  configured,
  labels,
  locale,
}: Readonly<{
  runtime?: AiRunSummary;
  configured: string;
  labels: AiRuntimeLabels;
  locale: Locale;
}>) {
  const [pinned, setPinned] = useState(false);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  // Hover and focus each open the popover, so closing it has to suppress both
  // until the reader leaves and comes back, or the button looks dead.
  const [dismissed, setDismissed] = useState(false);
  const popoverId = useId();
  const headingId = useId();
  const wrapper = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const popover = useRef<HTMLDivElement>(null);
  const open = !dismissed && (pinned || hovered || focused);
  const room = useRoomBelow(wrapper, popover, open);
  const region = useScrollRegion(popover, { labelledBy: headingId }, "block");

  // On the wrapper, so the pointer can travel onto the popover; native
  // listeners keep the wrapper a plain layout element rather than a control.
  useEffect(() => {
    const root = wrapper.current;
    if (!root) {
      return;
    }
    // The suppression lifts when the pointer or focus arrives, or once neither
    // is left holding the popover, so coming back opens it again.
    let hovering = false;
    let focusWithin = false;
    const enter = () => {
      hovering = true;
      setHovered(true);
      setDismissed(false);
    };
    const leave = () => {
      hovering = false;
      setHovered(false);
      if (!focusWithin) {
        setDismissed(false);
      }
    };
    // Focus inside the wrapper, so Tab can move from the chip into a popover
    // that scrolls without closing it.
    const focusIn = (event: FocusEvent) => {
      focusWithin = true;
      setFocused(true);
      const from = event.relatedTarget;
      if (!(from instanceof Node && root.contains(from))) {
        setDismissed(false);
      }
    };
    const focusOut = (event: FocusEvent) => {
      const next = event.relatedTarget;
      // A window losing focus keeps it on the element it was on.
      if (
        (next instanceof Node && root.contains(next)) ||
        root.contains(document.activeElement)
      ) {
        return;
      }
      focusWithin = false;
      setFocused(false);
      if (!hovering) {
        setDismissed(false);
      }
    };
    root.addEventListener("mouseenter", enter);
    root.addEventListener("mouseleave", leave);
    root.addEventListener("focusin", focusIn);
    root.addEventListener("focusout", focusOut);
    return () => {
      root.removeEventListener("mouseenter", enter);
      root.removeEventListener("mouseleave", leave);
      root.removeEventListener("focusin", focusIn);
      root.removeEventListener("focusout", focusOut);
    };
  }, []);

  // Keyed on `open`, not `pinned`: the popover focus holds open is the one
  // whose reader has no pointer to move away.
  useEffect(() => {
    if (!open) {
      return;
    }
    const close = () => {
      setPinned(false);
      setDismissed(true);
    };
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape" && !coveredByDialog(wrapper.current)) {
        // A popover about to be hidden would drop its focus onto the body.
        if (popover.current?.contains(document.activeElement)) {
          button.current?.focus();
        }
        close();
      }
    }
    function onPointerDown(event: PointerEvent) {
      const target = event.target;
      if (target instanceof Node && !wrapper.current?.contains(target)) {
        close();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("pointerdown", onPointerDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("pointerdown", onPointerDown);
    };
  }, [open]);

  const spend = runtime
    ? formatMicroUSD(runtime.estimated_cost_microusd, locale)
    : labels.awaiting;

  return (
    <div className="mw-aistat" ref={wrapper}>
      <button
        ref={button}
        type="button"
        className="mw-aistat-btn"
        aria-expanded={open}
        aria-controls={popoverId}
        // The spend is in the name: a bare label would hide the one figure the
        // button exists to show from a screen reader.
        aria-label={`${labels.chip}: ${spend}`}
        onClick={() => {
          // Toggles the pin, not the visible state: hover or focus may already
          // hold it open, and reading `open` would close what was just asked for.
          setPinned(!pinned);
          setDismissed(pinned);
        }}
      >
        <i aria-hidden />
        <strong>{spend}</strong>
        <ChevronDown className="mw-aistat-caret" aria-hidden />
      </button>
      <div
        ref={popover}
        className="mw-aistat-pop"
        id={popoverId}
        hidden={!open}
        style={room}
        {...region}
      >
        <p className="mw-aistat-h" id={headingId}>
          {labels.answering}
        </p>
        <dl className="mw-aistat-rows">
          <RuntimeRow label={labels.configured} value={configured} />
          <RuntimeRow
            label={labels.used}
            value={servedModels(runtime) || labels.awaiting}
          />
          <RuntimeRow
            label={labels.route}
            value={routes(runtime) || labels.unavailable}
          />
          <RuntimeRow
            label={labels.calls}
            value={
              runtime
                ? formatNumber(runtime.call_attempts, locale)
                : labels.unavailable
            }
          />
          <RuntimeRow
            label={labels.tokens}
            value={
              runtime
                ? formatNumber(runtime.tokens_in + runtime.tokens_out, locale)
                : labels.unavailable
            }
          />
          <RuntimeRow
            label={labels.latency}
            value={
              runtime
                ? `${formatNumber(runtime.latency_ms, locale)} ms`
                : labels.unavailable
            }
          />
          <RuntimeRow
            label={labels.estimatedCost}
            value={runtime ? spend : labels.unavailable}
            note={runtime?.unpriced_calls ? labels.partial : undefined}
          />
        </dl>
        <p className="mw-aistat-f">{labels.scope}</p>
      </div>
    </div>
  );
}

type RoomBelow = CSSProperties & Readonly<{ "--aistatRoom": string }>;

/** Every box between the chip and the body, nearest first. */
function ancestorsOf(from: HTMLElement): HTMLElement[] {
  const found: HTMLElement[] = [];
  for (
    let at = from.parentElement;
    at && at !== document.body;
    at = at.parentElement
  ) {
    found.push(at);
  }
  return found;
}

const CLIPS = new Set(["hidden", "clip", "auto", "scroll"]);

// Where the popover stops being painted: the viewport's foot, or higher where an
// ancestor clips (the onboarding stage hides its overflow above a phone's width).
function paintedFoot(from: HTMLElement): number {
  let foot = globalThis.innerHeight;
  for (const at of ancestorsOf(from)) {
    const style = getComputedStyle(at);
    if (CLIPS.has(style.overflowY) && style.display !== "contents") {
      const inner = at.getBoundingClientRect().bottom;
      foot = Math.min(foot, inner - Number.parseFloat(style.borderBottomWidth));
    }
  }
  return foot;
}

function roomBelow(anchor: HTMLElement): number {
  const chipFoot = anchor.getBoundingClientRect().bottom;
  return Math.max(paintedFoot(anchor) - chipFoot, 0);
}

// Not useAnchoredToTrigger: for a low trigger it measures the room ABOVE it,
// and this popover always opens below.
function useRoomBelow(
  anchor: RefObject<HTMLElement | null>,
  popover: RefObject<HTMLElement | null>,
  open: boolean,
): RoomBelow | undefined {
  const [room, setRoom] = useState<number | null>(null);
  // Every render while open, because the chip moves for reasons no event
  // reports: the band re-wrapping, a font arriving.
  useLayoutEffect(() => {
    if (open && anchor.current) {
      setRoom(roomBelow(anchor.current));
    }
  });
  useEffect(() => {
    if (!open) {
      return;
    }
    const measure = (event: Event) => {
      const moved = event.target;
      if (moved instanceof Node && popover.current?.contains(moved)) {
        return;
      }
      if (anchor.current) {
        setRoom(roomBelow(anchor.current));
      }
    };
    globalThis.addEventListener("resize", measure);
    globalThis.addEventListener("scroll", measure, true);
    // The chip moves when anything above it grows (the band wrapping, the text
    // size changing), and none of that renders this component or fires an event.
    const resized =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(() => {
            if (anchor.current) {
              setRoom(roomBelow(anchor.current));
            }
          });
    if (resized && anchor.current) {
      for (const box of [anchor.current, ...ancestorsOf(anchor.current)]) {
        resized.observe(box);
      }
    }
    return () => {
      resized?.disconnect();
      globalThis.removeEventListener("resize", measure);
      globalThis.removeEventListener("scroll", measure, true);
    };
  }, [open, anchor, popover]);
  return room === null ? undefined : { "--aistatRoom": `${room}px` };
}

function RuntimeRow({
  label,
  value,
  note,
}: Readonly<{ label: string; value: string; note?: string }>) {
  return (
    <div className="mw-aistat-r">
      <dt>{label}</dt>
      <dd>
        {value}
        {note && <small>{note}</small>}
      </dd>
    </div>
  );
}

function servedModels(runtime?: AiRunSummary) {
  return unique(
    (runtime?.models ?? []).map((entry) => entry.served_model).filter(Boolean),
  ).join(" + ");
}

function routes(runtime?: AiRunSummary) {
  return unique(
    (runtime?.models ?? []).map(
      (entry) => `${entry.task} · ${entry.tier} · ${entry.provider}`,
    ),
  ).join(" + ");
}

function unique(values: string[]) {
  return values.filter((value, index) => values.indexOf(value) === index);
}

// Not `formatMoney`: it rounds a fraction-of-a-cent read to $0.00. The locale
// stays the reader's, because a German reader writes 0,0043 $.
function formatMicroUSD(value: number, locale: Locale) {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: value > 0 && value < 10_000 ? 4 : 2,
    maximumFractionDigits: 6,
  }).format(value / 1_000_000);
}
