// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import type { ReactNode } from "react";
import { useT } from "../i18n";
import "./checklist.css";

/**
 * Numbered steps toward something working: connect a calendar, check the
 * hours, share the link. An ordered list, so the order is announced, with the
 * numbers drawn from a CSS counter rather than typed into each step.
 *
 * Not a wizard stepper: every step is on screen at once and the reader may do
 * them in any order. Not a tick control either; a step is done because the
 * product can see it is, never because somebody pressed it.
 */
export function Checklist({ children }: Readonly<{ children: ReactNode }>) {
  return <ol className="checklist">{children}</ol>;
}

export type ChecklistStepState = "done" | "current" | "open";

/**
 * One step. `current` is the one to do next and carries aria-current="step";
 * `open` is still to do. A step waiting on somebody else is `open` with a
 * caption saying who, and no action.
 */
export function ChecklistStep({
  state,
  title,
  caption,
  action,
  children,
}: Readonly<{
  state: ChecklistStepState;
  title: string;
  caption?: ReactNode;
  /** The one verb for this step, or a Badge on a done one. */
  action?: ReactNode;
  /** A body under the title for a step that needs more than a line. */
  children?: ReactNode;
}>) {
  const t = useT();
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
              reader as a word. */}
          <span className="sr-only">
            {t(state === "done" ? "checklist.stepDone" : "checklist.stepOpen")}
          </span>
        </span>
        {caption !== undefined && <span className="t-caption">{caption}</span>}
        {children !== undefined && (
          <div>{children}</div>
        )}
      </div>
      {action !== undefined && <div className="checklist-action">{action}</div>}
    </li>
  );
}
