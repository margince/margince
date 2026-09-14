// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { MoneyInput } from "../design-system/moneyinput";
import { formatMoney } from "../format/format";
import { monthlyEquivalent } from "../format/recurring";
import { useLocale, useT } from "../i18n";

/**
 * The recurring half of what an agreement is worth, and its monthly reading.
 *
 * Its own file rather than another block inside `contractform.tsx`, which is at
 * its length cap. It is rendered inside `ContractTermsFields`, directly under
 * the total value, because the two are one money value on the record: they
 * share a currency, and the server refuses either one stranded without it.
 *
 * The monthly figure below the input is a READING, never an input of its own.
 * A second editable field would make two ways to state one number, and the
 * annual one is what the record stores and what the paper says.
 */
export function ContractArrField({
  arrMinor,
  currency,
  onChangeMinor,
}: Readonly<{
  arrMinor: number;
  currency: string;
  onChangeMinor: (minor: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Only where there is something to divide AND a unit to read it in. A
  // monthly figure with no currency cannot be scaled, and a monthly nought
  // beside an unpriced agreement states a recurring value nobody agreed.
  const monthly =
    arrMinor > 0 && currency !== "" ? monthlyEquivalent(arrMinor) : null;
  return (
    <Field label={t("contracts.form.arr")}>
      {(props) => (
        <>
          <MoneyInput
            {...props}
            min={0}
            currency={currency}
            valueMinor={arrMinor}
            // An agreement may carry no recurring value at all, so an unpriced
            // one shows an empty field rather than a nought nobody typed.
            blankWhenZero
            onChangeMinor={onChangeMinor}
          />
          {monthly && (
            // Marked when the division lost something: twelve of an
            // approximate figure do not add back to the year, and a reader
            // who multiplies it should not be surprised by the difference.
            <p className="t-caption">
              {t("contracts.form.arrMonthly")}{" "}
              {monthly.approximate && `${t("deal.monthlyApproximate")} `}
              {formatMoney(monthly.monthlyMinor, currency, locale)}
            </p>
          )}
        </>
      )}
    </Field>
  );
}
