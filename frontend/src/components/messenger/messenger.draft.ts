// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";
export type MessengerDraft = Readonly<{
  text: string;
  request: string | null;
  retryAt?: string;
  acknowledged?: string;
}>;
function isDraft(value: unknown): value is MessengerDraft {
  return (
    typeof value === "object" &&
    value !== null &&
    "text" in value &&
    typeof value.text === "string" &&
    (!("acknowledged" in value) ||
      value.acknowledged === undefined ||
      typeof value.acknowledged === "string") &&
    "request" in value &&
    (value.request === null || typeof value.request === "string") &&
    (!("retryAt" in value) ||
      value.retryAt === undefined ||
      (typeof value.retryAt === "string" &&
        Number.isFinite(Date.parse(value.retryAt))))
  );
}
function draftStorage(member: string) {
  return { family: STORAGE_KEYS.messengerDraft, member };
}
export function readMessengerDraft(key: string): MessengerDraft {
  return (
    readStoredJson(draftStorage(key), (value) =>
      isDraft(value) ? value : null,
    ) ?? { text: "", request: null }
  );
}
export function keepMessengerDraft(key: string, value: MessengerDraft) {
  if (
    value.text === "" &&
    value.request === null &&
    !value.retryAt &&
    !value.acknowledged
  ) {
    removeStored(draftStorage(key));
    return;
  }
  writeStored(draftStorage(key), JSON.stringify(value));
}

export function applyMessengerAttempt(
  key: string,
  request: string,
  result: Readonly<{ state: string; retry_at?: string }>,
) {
  const current = readMessengerDraft(key);
  if (current.request !== request) return;
  if (result.state === "accepted")
    keepMessengerDraft(key, {
      text: "",
      request: null,
      acknowledged: current.acknowledged,
    });
  if (result.state === "failed")
    keepMessengerDraft(key, {
      text: current.text,
      request: null,
      retryAt: result.retry_at,
      acknowledged: current.acknowledged,
    });
}

export function reserveMessengerRetry(
  key: string,
  request: string,
  retryAt: string,
) {
  const current = readMessengerDraft(key);
  if (current.request === request)
    keepMessengerDraft(key, { ...current, request: null, retryAt });
}
