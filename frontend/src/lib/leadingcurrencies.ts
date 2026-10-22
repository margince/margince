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
 * The codes every currency picker leads with: the base currency, then the
 * currencies of the rate sheet. A form opens on the first of them, and sends
 * it when the field is left empty.
 *
 * The base alone while the sheet is pending or failed, so the order never
 * waits on the second read. The fallback four while the base is unknown.
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
      return data.data;
    },
  });
  if (!base) {
    return FALLBACK_LEADING_CURRENCIES;
  }
  return [base, ...(sheet.data ?? []).filter((code) => code !== base)];
}
