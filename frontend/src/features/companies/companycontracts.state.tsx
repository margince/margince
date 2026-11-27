// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { formatDateAbbrev } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

// What a contract row says about its dates and its place in a renewal chain.

type Contract = components["schemas"]["Contract"];

function useDay() {
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  return (date: string) => formatDateAbbrev(date, locale, readerZone);
}

// The term as the two dates that bound it. Absent dates say so in words,
// because a blank reads as "not loaded". An agreement entered from an invoice
// often has no recorded term.
export function ContractTerm({ contract }: Readonly<{ contract: Contract }>) {
  const t = useT();
  const on = useDay();
  if (!contract.starts_on && !contract.ends_on) {
    return <span>{t("contracts.noTerm")}</span>;
  }
  return (
    <span className="rec-term-dates">
      {contract.starts_on ? on(contract.starts_on) : t("contracts.openStart")}
      {" – "}
      {contract.ends_on ? on(contract.ends_on) : t("contracts.openEnd")}
    </span>
  );
}

/**
 * ContractTermState is where the row says what is actually true about its
 * dates, which is not always what its status says.
 *
 * The pending status change is the most surprising thing a reader can meet, so
 * it leads. A recorded cancellation comes next, whether or not its term has
 * begun. A row with a cancellation or a successor no longer says it renews.
 */
export function ContractTermState({
  contract,
}: Readonly<{ contract: Contract }>) {
  const t = useT();
  const on = useDay();

  // Its dates have run out and nobody has moved the status. Saying only
  // "active" here would present an approval queue as a live agreement.
  if (contract.term_state === "ended" && contract.status === "active") {
    return (
      <span className="t-caption">{t("contracts.endedPendingStatus")}</span>
    );
  }
  if (contract.cancellation_effective_on) {
    const when = on(contract.cancellation_effective_on);
    return (
      <Badge tone="warning">
        {contract.cancellation_notice_on
          ? t("contracts.cancelledToNotice", {
              when,
              notice: on(contract.cancellation_notice_on),
            })
          : t("contracts.cancelledTo", { when })}
      </Badge>
    );
  }
  if (contract.renewal_on && !contract.superseded_by_id) {
    return (
      <span className="t-caption">
        {t("contracts.renewsOn", { when: on(contract.renewal_on) })}
      </span>
    );
  }
  return null;
}

// The agreement that replaced this one. Its title comes from the rows the card
// holds; a successor the card does not hold is still named as one.
export function ContractSuccessor({
  contract,
  successorTitle,
}: Readonly<{ contract: Contract; successorTitle?: string }>) {
  const t = useT();
  if (!contract.superseded_by_id) {
    return null;
  }
  return (
    <span className="t-caption">
      {successorTitle
        ? t("contracts.replacedBy", { title: successorTitle })
        : t("contracts.replacedByUnnamed")}
    </span>
  );
}
