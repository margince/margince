// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessengerUris } from "../format/contacturi";
import { useInstallationSettings } from "./uploadlimit";

type InstallationSettings = components["schemas"]["InstallationSettings"];

export type Messenger = keyof MessengerUris;

/** The messengers in the order a phone number lists them. */
export const MESSENGERS = [
  "whatsapp",
  "telegram",
  "zalo",
] as const satisfies readonly Messenger[];

/** Each messenger's own name, which no language translates. */
export const MESSENGER_NAME = {
  whatsapp: "WhatsApp",
  telegram: "Telegram",
  zalo: "Zalo",
} as const satisfies Record<Messenger, string>;

/** The installation setting that turns each messenger's link on. */
export const MESSENGER_SETTING = {
  whatsapp: "call_whatsapp",
  telegram: "call_telegram",
  zalo: "call_zalo",
} as const satisfies Record<Messenger, keyof InstallationSettings>;

/** Whether the installation shows `messenger`'s chat link. */
export function messengerOn(
  settings: Partial<InstallationSettings>,
  messenger: Messenger,
): boolean {
  // A server older than these settings omits them, and its links cannot have
  // been turned off.
  return settings[MESSENGER_SETTING[messenger]] !== false;
}

/** The messengers whose chat links this installation shows beside a number. */
export function useCallMessengers(): readonly Messenger[] {
  const { data, isError } = useInstallationSettings();
  if (data) {
    return MESSENGERS.filter((messenger) => messengerOn(data, messenger));
  }
  // A custom role may read contacts without reading these settings; it still
  // gets the installation default, which is on.
  return isError ? MESSENGERS : [];
}
