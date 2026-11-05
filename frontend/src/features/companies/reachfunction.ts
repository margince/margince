// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type ReachFunction = components["schemas"]["LinkedInReachFunction"];
type ReachFunctionCount = components["schemas"]["LinkedInReachFunctionCount"];

// A Record, so a function the contract adds fails the build until it is named.
const FUNCTION_LABEL: Record<ReachFunction, MessageKey> = {
  management: "companyReach.function.management",
  it: "companyReach.function.it",
  finance: "companyReach.function.finance",
  sales: "companyReach.function.sales",
  marketing: "companyReach.function.marketing",
  hr: "companyReach.function.hr",
  operations: "companyReach.function.operations",
  procurement: "companyReach.function.procurement",
};

/** "IT · 3": a function and how many of a colleague's connections work in it. */
export function reachFunctionCount(
  t: Translator,
  locale: Locale,
  held: ReachFunctionCount,
): string {
  return t("companyReach.functionCount", {
    function: t(FUNCTION_LABEL[held.function]),
    connections: formatNumber(held.connections, locale),
  });
}
