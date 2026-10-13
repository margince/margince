// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Step } from "./connect-flow-draft";
import "./connect-flow.css";

// The connect trail, drawn twice: inside the connect dialog, and as the strip
// on the empty Mail and calendar card that says what that dialog will ask.
// Both callers are on Connections, so it stays a screen file, not a primitive.

const STEP_LABELS: readonly MessageKey[] = [
  "connectFlow.stepAccount",
  "connectFlow.stepBringIn",
  "connectFlow.stepSignIn",
];

const STRIP_LABELS: readonly MessageKey[] = [
  "connectors.strip.pick",
  "connectors.strip.tick",
  "connectors.strip.signIn",
];

type TrailState = "done" | "current" | "todo";

/** Where the reader is in the three steps. A trail rather than a control: the
 *  steps are walked with Back and Continue, so none of it is pressable. */
export function StepTrail({ step }: Readonly<{ step: Step }>) {
  const t = useT();
  return (
    <ol className="connect-flow-trail" aria-label={t("connectFlow.progress")}>
      {STEP_LABELS.map((key, index) => {
        const number = index + 1;
        const state: TrailState =
          number < step ? "done" : number === step ? "current" : "todo";
        return (
          <li
            key={key}
            className="connect-flow-step"
            data-state={state}
            aria-current={state === "current" ? "step" : undefined}
          >
            <span className="connect-flow-num" aria-hidden="true">
              {state === "done" ? <Check /> : number}
            </span>
            {state === "done" ? (
              <>
                <span aria-hidden="true">{t(key)}</span>
                <span className="sr-only">
                  {t("connectFlow.stepDone", { step: t(key) })}
                </span>
              </>
            ) : (
              t(key)
            )}
          </li>
        );
      })}
    </ol>
  );
}

/** What connecting will ask, before anything is asked: the same steps, none of
 *  them started, ending on what happens without the reader. */
export function ConnectStrip() {
  const t = useT();
  return (
    <ol className="connect-flow-trail" aria-label={t("connectors.strip.label")}>
      {STRIP_LABELS.map((key, index) => (
        <li key={key} className="connect-flow-step" data-state="todo">
          <span className="connect-flow-num" aria-hidden="true">
            {index + 1}
          </span>
          {t(key)}
        </li>
      ))}
      <li className="connect-flow-step" data-state="todo">
        <span className="connect-flow-num" aria-hidden="true">
          <Check />
        </span>
        {t("connectors.strip.syncs")}
      </li>
    </ol>
  );
}
