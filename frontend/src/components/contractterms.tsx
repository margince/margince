// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatMoney, formatNumber } from "../format/format";
import { monthlyEquivalent } from "../format/recurring";
import { type Locale, useLocale, useT } from "../i18n";

type Contract = components["schemas"]["Contract"];

/**
 * How long the customer has to pay, and what the agreement is worth per year.
 * The company's Contracts panel and the project's Contracts card both draw
 * this, so the line reads the same on both pages. Caption siblings, not a
 * block: each host shows an agreement as one line under its name.
 */
export function ContractTerms({ contract }: Readonly<{ contract: Contract }>) {
  const { locale } = useLocale();
  return (
    <>
      <PaymentTerm days={contract.payment_term_days} />
      <ContractArr contract={contract} locale={locale} />
    </>
  );
}

/** Whether `ContractTerms` draws anything, for a host that leaves out an empty line. */
export function hasContractTerms(contract: Contract): boolean {
  return contract.payment_term_days != null || pricedArr(contract) != null;
}

// An annual figure is shown only with the currency that prices it.
function pricedArr(
  contract: Contract,
): { arr: number; currency: string } | null {
  const arr = contract.arr_minor;
  const currency = contract.currency;
  return arr == null || !currency ? null : { arr, currency };
}

/**
 * How long the customer has to pay.
 *
 * ZERO IS NOT BLANK, and the whole helper turns on that. The form's own hint
 * says it: 0 means the invoice is due on receipt, and no value means nobody
 * has agreed terms at all. Rendering the first as an empty caption would state
 * that an agreement demanding immediate payment has no terms — the opposite of
 * what somebody negotiated.
 */
function PaymentTerm({ days }: Readonly<{ days?: number | null }>) {
  const t = useT();
  const { locale } = useLocale();
  if (days == null) {
    return null;
  }
  return (
    <span className="t-caption">
      {days === 0
        ? t("contracts.terms.onReceipt")
        : // A day count is a QUANTITY, so it is grouped the way the reader
          // reads every other figure on the page rather than as a bare
          // numeral. A 120-day term reads 120 here and 120 in German too,
          // but the rule is the same one the rest of the product keeps.
          t("contracts.terms.net", { days: formatNumber(days, locale) })}
    </span>
  );
}

/**
 * The recurring half of what the agreement is worth, with its monthly reading.
 *
 * The monthly figure is marked when the division lost something: twelve of an
 * approximate monthly figure do not add back to the year, and a reader who
 * multiplies it should not be surprised by the difference. Same rule the
 * contract form's own ARR field keeps.
 *
 * Withheld with the currency, never without it. An annual figure with nothing
 * to price it in is a number a reader will read in whatever currency the row
 * beside it showed.
 */
function ContractArr({
  contract,
  locale,
}: Readonly<{ contract: Contract; locale: Locale }>) {
  const t = useT();
  const priced = pricedArr(contract);
  if (!priced) {
    return null;
  }
  const { arr, currency } = priced;
  const monthly = arr > 0 ? monthlyEquivalent(arr) : null;
  return (
    <span className="t-caption">
      {t("contracts.terms.arr", { amount: formatMoney(arr, currency, locale) })}
      {monthly && (
        <>
          {" "}
          {t("contracts.terms.monthly", {
            amount: `${monthly.approximate ? `${t("deal.monthlyApproximate")} ` : ""}${formatMoney(monthly.monthlyMinor, currency, locale)}`,
          })}
        </>
      )}
    </span>
  );
}
