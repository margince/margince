// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "./atoms";

/**
 * The four things a status can say. A result nobody acts on (Lost, Rejected,
 * Invited, Deactivated) is `neutral`, so red stays for something that broke.
 */
export const STATUS_KINDS = [
  "done",
  "neutral",
  "attention",
  "failure",
] as const;
export type StatusKind = (typeof STATUS_KINDS)[number];

type StatusTone = "success" | "default" | "warning" | "danger";

// Blue (`info`, `accent`) is never a status: it read as "the next thing to press".
const TONE_OF: Readonly<Record<StatusKind, StatusTone>> = {
  done: "success",
  neutral: "default",
  attention: "warning",
  failure: "danger",
};

export function statusTone(kind: StatusKind): StatusTone {
  return TONE_OF[kind];
}

/** One status as a Badge; `live` adds the dot for running or syncing. */
export function StatusBadge({
  kind,
  label,
  live,
}: Readonly<{ kind: StatusKind; label: ReactNode; live?: boolean }>) {
  return (
    <Badge tone={statusTone(kind)} live={live}>
      {label}
    </Badge>
  );
}
