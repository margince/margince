// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Choice, ChoiceList } from "../design-system/choicelist";
import { useT } from "../i18n";
import { CALL_OUTCOMES, type CallOutcome } from "../lib/activitybody";
import { CALL_OUTCOME_LABEL } from "../lib/calloutcome";
import { labelFor } from "../lib/labelfor";

/** How the call ended, picked with one click. */
export function CallOutcomeField({
  value,
  onChange,
}: Readonly<{
  value: CallOutcome | "";
  onChange: (outcome: CallOutcome | "") => void;
}>) {
  const t = useT();
  // "No outcome" first: the question always holds an answer, and a call saved
  // without an outcome is a whole call.
  const choices: Choice<CallOutcome | "">[] = [
    { value: "", label: t("log.callOutcomeNone") },
    ...CALL_OUTCOMES.map((outcome) => ({
      value: outcome,
      label: t(labelFor(CALL_OUTCOME_LABEL, outcome)),
    })),
  ];
  return (
    <ChoiceList
      legend={t("log.callOutcome")}
      layout="row"
      choices={choices}
      value={value}
      onChange={onChange}
    />
  );
}
