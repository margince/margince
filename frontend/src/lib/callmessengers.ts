// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessengerUris } from "../format/contacturi";
import { problemStatus } from "./problem";
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
  const { data, error } = useInstallationSettings();
  if (data) {
    return MESSENGERS.filter((messenger) => messengerOn(data, messenger));
  }
  // A role refused this read still gets the installation default, which is on.
  // Any other failure shows none, or a link an admin turned off would return.
  return problemStatus(error) === 403 ? MESSENGERS : [];
}
