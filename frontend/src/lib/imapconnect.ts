// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import {
  type FieldLabels,
  problemCodeOf,
  problemMessageOf,
  type RefusalSentences,
  refusalSentences,
} from "./problem";

// What a personal and a shared IMAP connect check and say alike: the port
// bounds, and the server's refusals of the login it tried.

export type Range = Readonly<{ min: number; max: number }>;

export const PORT_RANGE: Range = { min: 1, max: 65535 };

export function inRange(value: number, { min, max }: Range): boolean {
  return Number.isInteger(value) && value >= min && value <= max;
}

// No sentence echoes the submitted host, and the server does not send it back
// either.
const SERVER_SENTENCES: Readonly<Record<string, MessageKey>> = {
  imap_login_rejected: "connectors.imapLoginRejected",
  imap_unreachable: "connectors.imapUnreachable",
  smtp_login_rejected: "connectors.smtpLoginRejected",
  smtp_unreachable: "connectors.smtpUnreachable",
  smtp_address_invalid: "connectors.smtpAddressInvalid",
};

function serverSentence(error: unknown): MessageKey | undefined {
  return SERVER_SENTENCES[problemCodeOf(error) ?? ""];
}

// A refused connect as one sentence: the mail server's condition in its own
// words, any other failure the way failures read everywhere else.
export function imapErrorMessage(
  error: unknown,
  t: (key: MessageKey) => string,
): string {
  const key = serverSentence(error);
  return key ? t(key) : problemMessageOf(error, t);
}

// A refused connect split between the form's fields and its foot. The mail
// server's condition names no field, so it stays at the foot.
export function imapRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
  labels: FieldLabels,
): RefusalSentences {
  const key = serverSentence(error);
  return key
    ? { byField: new Map(), rest: t(key) }
    : refusalSentences(error, t, labels);
}
