// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import { readStored, type StorageKey, writeStored } from "./storage";

const OFF = "off";
const ON = "on";

export type DeviceSwitch = Readonly<{
  read: () => boolean;
  write: (next: boolean) => void;
  subscribe: (listener: () => void) => () => void;
}>;

/** ON until a reader turns it off. Held once read, so a browser that refuses
 *  storage still shows the flip until the next page. */
export function deviceSwitch(key: StorageKey): DeviceSwitch {
  let shown: boolean | null = null;
  const listeners = new Set<() => void>();
  const read = () => {
    shown ??= readStored(key) !== OFF;
    return shown;
  };
  const write = (next: boolean) => {
    shown = next;
    writeStored(key, next ? ON : OFF);
    for (const listener of listeners) {
      listener();
    }
  };
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  };
  return { read, write, subscribe };
}

export function useDeviceSwitch(choice: DeviceSwitch): boolean {
  return useSyncExternalStore(choice.subscribe, choice.read, choice.read);
}
