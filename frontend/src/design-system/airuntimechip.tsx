// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import type { components } from "../api/schema";
import { formatNumber, INTL_LOCALE } from "../format/format";
import type { Locale } from "../i18n";
import "./airuntimechip.css";
import { coveredByDialog } from "./dialogfocus";

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
  const wrapper = useRef<HTMLDivElement>(null);
  const open = !dismissed && (pinned || hovered || focused);

  // On the wrapper, so the pointer can travel onto the popover; native
  // listeners keep the wrapper a plain layout element rather than a control.
  useEffect(() => {
    const root = wrapper.current;
    if (!root) {
      return;
    }
    const enter = () => {
      setHovered(true);
      setDismissed(false);
    };
    const leave = () => {
      setHovered(false);
      setDismissed(false);
    };
    root.addEventListener("mouseenter", enter);
    root.addEventListener("mouseleave", leave);
    return () => {
      root.removeEventListener("mouseenter", enter);
      root.removeEventListener("mouseleave", leave);
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
        onFocus={() => setFocused(true)}
        onBlur={() => {
          setFocused(false);
          // Leaving resets the suppression, so coming back opens again.
          setDismissed(false);
        }}
      >
        <i aria-hidden />
        <strong>{spend}</strong>
        <ChevronDown className="mw-aistat-caret" aria-hidden />
      </button>
      <div className="mw-aistat-pop" id={popoverId} hidden={!open}>
        <p className="mw-aistat-h">{labels.answering}</p>
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
