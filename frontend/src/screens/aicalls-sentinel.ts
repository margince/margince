// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

const SENTINEL_LABEL: Readonly<Record<string, MessageKey>> = {
  provider_quota: "aicalls.sentinel.provider_quota",
  provider_throttled: "aicalls.sentinel.provider_throttled",
  provider_refused: "aicalls.sentinel.provider_refused",
  provider_error: "aicalls.sentinel.provider_error",
  timeout: "aicalls.sentinel.timeout",
  output_withheld: "aicalls.sentinel.output_withheld",
  output_rejected: "aicalls.sentinel.output_rejected",
  request_rejected: "aicalls.sentinel.request_rejected",
};

/** A failure code in words. The server adds codes, so an unknown one reads as some failure. */
export function sentinelLabel(sentinel: string, t: Translator): string {
  const key = SENTINEL_LABEL[sentinel];
  return key ? t(key) : t("aicalls.outcome.failed");
}
