// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSyncExternalStore } from "react";
import type { components } from "../api/schema";
import { useMe } from "./session";

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

export function healthScope(
  me: components["schemas"]["MeResponse"] | undefined,
) {
  return me
    ? JSON.stringify([me.user.id, me.workspace_name, me.authorization])
    : undefined;
}
