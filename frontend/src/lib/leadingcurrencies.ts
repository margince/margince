// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import {
  FALLBACK_LEADING_CURRENCIES,
  type LeadingCurrencies,
} from "../format/currencychoices";
import { throwProblem } from "./problem";
import { useInstallationSettings } from "./uploadlimit";

// Under the settings screen's `["fx-rates"]` key, so saving a rate there also
// refreshes every picker.
export const FX_RATE_CURRENCIES_KEY = ["fx-rates", "currencies"] as const;

/**
 * The codes every currency picker leads with: the base, then the rate sheet's
 * currencies. A form opens on the first, and sends it for an empty field.
 *
 * The sheet read also names the base, for a role refused the settings read.
 * The base alone while the sheet is pending or failed; the fallback four while
 * no read has named a base.
 */
export function useLeadingCurrencies(): LeadingCurrencies {
  const base = useInstallationSettings().data?.base_currency;
  const sheet = useQuery({
    queryKey: FX_RATE_CURRENCIES_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/fx-rates/currencies");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
  const leader = base ?? sheet.data?.base_currency;
  if (!leader) {
    return FALLBACK_LEADING_CURRENCIES;
  }
  return [
    leader,
    ...(sheet.data?.data ?? []).filter((code) => code !== leader),
  ];
}
