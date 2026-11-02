// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Reading = Pick<
  components["schemas"]["HealthSource"],
  "code" | "condition"
> & { availability?: components["schemas"]["HealthSource"]["availability"] };
export function healthTone(reading: Reading) {
  if (
    reading.availability === "failed" ||
    reading.condition === "technical" ||
    ["read_failed", "queue_unverified"].includes(reading.code)
  )
    return "danger";
  if (["retrying", "waiting_human", "unverified"].includes(reading.condition))
    return "warning";
  if (
    reading.availability === "available" &&
    ["clear", "recovered"].includes(reading.condition)
  )
    return "success";
  return "default";
}
