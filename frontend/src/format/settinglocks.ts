// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

type SettingLock = components["schemas"]["SettingLock"];
type Source = SettingLock["origin"]["source"];

export type FieldLock =
  | { disabled: true; reason: string }
  | { disabled: false };

// Only a boot source locks a setting. A saved, default or absent value is the
// administrator's to change, so its source has no entry here.
const LOCKED_BY: Partial<Record<Source, MessageKey>> = {
  flag: "settings.lockedBy.flag",
  env: "settings.lockedBy.env",
  env_file: "settings.lockedBy.env_file",
  config_file: "settings.lockedBy.config_file",
};

// The wording for a file source whose origin names no file.
const LOCKED_BY_UNNAMED_FILE: Partial<Record<Source, MessageKey>> = {
  env_file: "settings.lockedBy.env_file_unnamed",
  config_file: "settings.lockedBy.config_file_unnamed",
};

/** Whether the setting `key` is set outside the app, and where to change it. */
export function lockFor(
  locks: readonly SettingLock[] | undefined,
  key: string,
  t: Translator,
): FieldLock {
  const origin = locks?.find((lock) => lock.key === key)?.origin;
  const message = origin && LOCKED_BY[origin.source];
  if (!(origin && message)) {
    return { disabled: false };
  }
  if (!origin.file) {
    const unnamed = LOCKED_BY_UNNAMED_FILE[origin.source] ?? message;
    return { disabled: true, reason: t(unnamed, { name: origin.name }) };
  }
  return {
    disabled: true,
    reason: t(message, { name: origin.name, file: origin.file }),
  };
}

/**
 * The sentence that refuses the control for `key`, or `otherwise`. A lock binds
 * every role, administrators included, so it speaks before a role refusal does.
 */
export function lockReasonOr(
  locks: readonly SettingLock[] | undefined,
  key: string,
  t: Translator,
  otherwise: string | undefined,
): string | undefined {
  const lock = lockFor(locks, key, t);
  return lock.disabled ? lock.reason : otherwise;
}

/**
 * A control's `aria-describedby` plus the id of the one reason a locked form
 * draws, so every refused control in it is told why.
 */
export function describedWith(
  own: string | undefined,
  reasonId: string | undefined,
): string | undefined {
  return [own, reasonId].filter(Boolean).join(" ") || undefined;
}

/**
 * The id a form's one drawn refusal takes, which every refused control points
 * at. Undefined while nothing refuses, so no control points at a missing node.
 */
export function useRefusalId(refusal: string | undefined): string | undefined {
  const id = useId();
  return refusal === undefined ? undefined : id;
}
