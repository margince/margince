// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useT } from "../../i18n";
import {
  type FieldLabels,
  type RefusalSentences,
  refusalSentences,
} from "../../lib/problem";
import type { EditedFact, InstallationPart } from "./installation-settings";

const FORECAST_FACTS: readonly EditedFact[] = [
  "forecast_forward_measure",
  "forecast_likely_threshold",
];

// The server names a refused setting by its key (`installation.timezone`) and
// the form keys its fields by the patch name (`timezone`). Labels only for the
// facts this dialog draws, so a refusal naming any other field reaches the foot.
function drawnLabels(labels: FieldLabels, part: InstallationPart): FieldLabels {
  const drawn = Object.entries(labels).filter(
    ([fact]) =>
      FORECAST_FACTS.some((forecast) => forecast === fact) ===
      (part === "forecast"),
  );
  return Object.fromEntries(
    drawn.flatMap(([fact, label]) => [
      [fact, label],
      [`installation.${fact}`, label],
    ]),
  );
}

export function installationRefusal(
  error: unknown,
  t: ReturnType<typeof useT>,
  labels: FieldLabels,
  part: InstallationPart,
): RefusalSentences {
  const { byField, rest } = refusalSentences(
    error,
    t,
    drawnLabels(labels, part),
  );
  return {
    byField: new Map(
      [...byField].map(([field, sentence]) => [
        field.replace(/^installation\./, ""),
        sentence,
      ]),
    ),
    rest,
  };
}
