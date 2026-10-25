import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { formatDate, formatMoneyOrAbsent } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { isEntityKind } from "../../lib/entity";
import { useRecordZone } from "../../lib/recordzone";

type InputCheck = components["schemas"]["InputCheck"];

// Which deal a finding is about, as a link. An older server sends no subject,
// and a kind with no screen is left out, as the Worklist's rows leave it.
export function SubjectCell({
  check,
  newTab = false,
}: Readonly<{ check: InputCheck; newTab?: boolean }>) {
  if (!check.subject || !isEntityKind(check.subject.type)) {
    return null;
  }
  return (
    <EntityRef
      kind={check.subject.type}
      id={check.subject.id}
      name={check.subject.label}
      newTab={newTab}
    />
  );
}

// A finding's own name, keyed by the check that noticed it.
//
// An unknown type falls back to a generic line rather than the raw key. A rule
// newer than this screen still reads as a finding, not `close_pushed_v2`.
export function checkLabel(
  type: string,
): Parameters<ReturnType<typeof useT>>[0] {
  const known: Record<string, string> = {
    unlinked_signal: "review.unlinkedSignal",
    close_past: "review.closePast",
    close_unconfirmed: "review.closeUnconfirmed",
    close_pushed: "review.closePushed",
    amount_vs_offer: "review.amountVsOffer",
    amount_vs_contract: "review.amountVsContract",
    no_next_step: "review.noNextStep",
    no_economic_buyer: "review.noEconomicBuyer",
    buyer_silent: "review.buyerSilent",
    commit_unpriced: "review.commitUnpriced",
    stage_exit_unmet: "review.stageExitUnmet",
  };
  return (known[type] ?? "review.unknownCheck") as Parameters<
    ReturnType<typeof useT>
  >[0];
}

// CheckFacts is what an answer is about, as the table row shows it. That is the
// finding, its deal, the money at stake and when it was first seen. The answer
// sheet covers the table, so without these the reader answers blind.
export function CheckFacts({ check }: Readonly<{ check: InputCheck }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <FieldGrid>
      <FieldRow label={t("review.colFinding")}>
        {t(checkLabel(check.type))}
      </FieldRow>
      {check.subject && (
        <FieldRow label={t("review.colDeal")}>
          <SubjectCell check={check} newTab />
        </FieldRow>
      )}
      <FieldRow label={t("review.colAtStake")}>
        <span className="t-num">
          {formatMoneyOrAbsent(
            check.affected_minor ?? null,
            check.currency ?? "",
            locale,
          )}
        </span>
      </FieldRow>
      <FieldRow label={t("review.colSeenSince")}>
        {formatDate(check.first_seen_at, locale, zone)}
      </FieldRow>
    </FieldGrid>
  );
}
