// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import { refusedLock } from "./installation-settings.currency";
import {
  type FieldLabels,
  problemFieldErrorsOf,
  type RefusalSentences,
  refusalSentences,
} from "./problem";

const SETTING_KEY = /^installation\./;

// The server names a refused setting by its key (`installation.timezone`) and
// a form keys its fields by the patch name (`timezone`). Pass labels only for
// the fields the form draws, so a refusal naming any other reaches the foot.
export function installationRefusal(
  error: unknown,
  t: Translator,
  labels: FieldLabels,
): RefusalSentences {
  const byKey = Object.fromEntries(
    Object.entries(labels).flatMap(([fact, label]) => [
      [fact, label],
      [`installation.${fact}`, label],
    ]),
  );
  const { byField, rest } = refusalSentences(error, t, byKey);
  return {
    byField: new Map(
      [...byField].map(([field, sentence]) => [
        field.replace(SETTING_KEY, ""),
        sentence,
      ]),
    ),
    rest,
  };
}

// A refusal that words the lock stands in for the base currency's hint, which
// states the same lock.
export function refusalWordsLock(error: unknown): boolean {
  return problemFieldErrorsOf(error).some(
    (fault) => refusedLock(fault) !== undefined,
  );
}
