// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import { healthScope } from "./healthscopekey";

import { useMe } from "./session";

export { healthScope } from "./healthscopekey";

function subscribeVisibility(changed: () => void) {
  document.addEventListener("visibilitychange", changed);
  return () => document.removeEventListener("visibilitychange", changed);
}

export function useHealthVisible() {
  return useSyncExternalStore(
    subscribeVisibility,
    () => document.visibilityState !== "hidden",
    () => false,
  );
}

export function useHealthScope() {
  return healthScope(useMe().data);
}
