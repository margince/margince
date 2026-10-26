// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import { createContext, type ReactNode, useContext, useId } from "react";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import { Badge } from "./atoms";
import "./checklist.css";

type ChecklistVariant = "list" | "inline";

const VariantContext = createContext<ChecklistVariant>("list");

/**
 * Numbered steps toward something working, as an ordered list so the order is
 * announced. Numbers come from a CSS counter.
 *
 * Not a wizard stepper: every step is on screen and any order works. Not a
 * tick control either: a step is done because the product can see it is.
 *
 * `inline` is the one-line trail of a flow walked with Back and Continue:
 * marks and titles only.
 */
export function Checklist({
  count,
  variant = "list",
  label,
  children,
}: Readonly<{
  /** Drawn as a caption above the list, "2 of 4 done". */
  count?: Readonly<{ done: number; total: number }>;
  variant?: ChecklistVariant;
  /** Names the list for a screen reader where no heading does. */
  label?: string;
  children: ReactNode;
}>) {
  const t = useT();
  const countId = useId();
  const { locale } = useLocale();
  const list = (
    <ol
      className={`checklist checklist-${variant}`}
      aria-label={label}
      aria-describedby={count === undefined ? undefined : countId}
    >
      <VariantContext.Provider value={variant}>
        {children}
      </VariantContext.Provider>
    </ol>
  );
  if (count === undefined) {
    return list;
  }
  return (
    <div className="checklist-counted">
      <p id={countId} className="checklist-count t-caption">
        {t("checklist.count", {
          done: formatNumber(count.done, locale),
          total: formatNumber(count.total, locale),
        })}
      </p>
      {list}
    </div>
  );
}

export type ChecklistStepState = "done" | "current" | "open" | "waiting";

type StepState =
  | {
      state: Exclude<ChecklistStepState, "waiting">;
      /** The one verb for this step, a `primary` Button on the current one,
       *  or a Badge on a done one. */
      action?: ReactNode;
      waitingLabel?: never;
    }
  | {
      /** Nothing the reader can do: somebody else has to act first. */
      state: "waiting";
      action?: never;
      /** The badge's words, when "Waiting" says less than the caller can. */
      waitingLabel?: string;
    };

/**
 * One step. `current` is the one to do next and carries aria-current="step";
 * `open` is still to do. A step waiting on somebody else is `waiting`, with a
 * caption saying who; it shows a grey Badge in place of an action.
 */
export function ChecklistStep({
  state,
  title,
  caption,
  action,
  waitingLabel,
  children,
}: Readonly<
  {
    title: string;
    caption?: ReactNode;
    /** A body under the title for a step that needs more than a line. */
    children?: ReactNode;
  } & StepState
>) {
  const t = useT();
  const inline = useContext(VariantContext) === "inline";
  const waitingWords = waitingLabel ?? t("checklist.stepWaiting");
  const trailing = state === "waiting" ? <Badge>{waitingWords}</Badge> : action;
  const showTrailing = !inline && trailing !== undefined;
  return (
    <li
      className={`checklist-step checklist-step-${state}`}
      aria-current={state === "current" ? "step" : undefined}
    >
      <span className="checklist-mark" aria-hidden="true">
        {state === "done" && <Check size={14} strokeWidth={2.5} />}
      </span>
      <div className="checklist-text">
        <span className="checklist-title t-name">
          {title}{" "}
          {/* The mark is colour and a glyph; the state reaches a screen
              reader as a word, once, so not again beside a waiting badge. */}
          {!(state === "waiting" && showTrailing) && (
            <span className="sr-only">{stateWord(state, waitingWords, t)}</span>
          )}
        </span>
        {!inline && caption !== undefined && (
          <span className="t-caption">{caption}</span>
        )}
        {!inline && children !== undefined && <div>{children}</div>}
      </div>
      {showTrailing && <div className="checklist-action">{trailing}</div>}
    </li>
  );
}

function stateWord(
  state: ChecklistStepState,
  waitingWords: string,
  t: Translator,
): string {
  if (state === "waiting") {
    return waitingWords;
  }
  return t(state === "done" ? "checklist.stepDone" : "checklist.stepOpen");
}
