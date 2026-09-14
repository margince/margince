// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge } from "../design-system/atoms";
import { PanelRow } from "../design-system/panel";
import {
  formatDate,
  formatDateAbbrev,
  formatMoney,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { monthlyEquivalent } from "../format/recurring";
import { type Locale, useLocale, useT } from "../i18n";

type Contract = components["schemas"]["Contract"];

/**
 * The two terms an agreement carries that no read surface showed until now:
 * how long the customer has to pay, and what the agreement is worth per year.
 *
 * Both write fine from the contract form and the renewal form, and neither
 * appeared anywhere afterwards — a reader could record a payment term and then
 * had no way to see it again without reopening the form that wrote it.
 *
 * Drawn as caption siblings rather than as a block, because both hosts render
 * an agreement as one quiet line under its name (the company's contracts panel
 * and the project's contracts card), and a block would break that line in two.
 * Each half renders only when it has something to say, so an agreement with
 * neither recorded is unchanged.
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
  const arr = contract.arr_minor;
  const currency = contract.currency;
  if (arr == null || !currency) {
    return null;
  }
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

/**
 * One agreement on the project's contracts card.
 *
 * Lives here rather than in `projectsections.tsx` so the terms above and the
 * row that draws them are read together — and because that file is at its
 * length cap, which is what a cap is for: the next fact an agreement carries
 * goes where agreements are rendered, not onto the end of a file about
 * everything a project has.
 */
export function ProjectContractRow({
  contract,
  locale,
}: Readonly<{ contract: Contract; locale: Locale }>) {
  const recordZone = useRecordZone();
  return (
    <PanelRow className="project-row">
      <span>{contract.title}</span>
      <span className="project-row-meta t-caption">
        <Badge>{contract.status}</Badge>
        <span className="t-mono">
          {formatMoneyOrAbsent(contract.value_minor, contract.currency, locale)}
        </span>
        {contract.ends_on && (
          <span>{formatDateAbbrev(contract.ends_on, locale, recordZone)}</span>
        )}
        <ContractTerms contract={contract} />
      </span>
    </PanelRow>
  );
}

// The term as the two dates that bound it. Absent dates say so in words: a
// blank column reads as "not loaded", and an agreement whose term nobody
// recorded is a real and common state — it is entered from an invoice as
// often as from the paper.
export function ContractTerm({ contract }: Readonly<{ contract: Contract }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const on = (date: string) => formatDate(date, locale, recordZone);
  if (!contract.starts_on && !contract.ends_on) {
    return <span className="t-caption">{t("contracts.noTerm")}</span>;
  }
  return (
    <span className="rec-term-dates">
      {contract.starts_on ? on(contract.starts_on) : t("contracts.openStart")}
      {" – "}
      {contract.ends_on ? on(contract.ends_on) : t("contracts.openEnd")}
    </span>
  );
}
