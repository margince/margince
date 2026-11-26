// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { InfoTip } from "../../design-system/infotip";
import { useLocale, useT } from "../../i18n";
import { useDealMarginceChanges } from "../../lib/marginchanges";
import { receiptReason } from "../../lib/receiptcopy";
import { useViewerId } from "../../lib/session";
import { probabilityReason } from "./dealprobability";
import type { Deal } from "./deals";

type Receipt = components["schemas"]["Receipt"];

// The two Margince changes that set a deal's own figure: a verdict on the
// probability, and a close-date sweep that also lowered the forecast.
function setsProbability(receipt: Receipt): boolean {
  if (receipt.undo?.reversed) {
    return false;
  }
  return (
    receipt.kind === "deal_probability" ||
    receipt.close_date_change?.forecast_changed === true
  );
}

type HintProps = Readonly<{
  deal: Deal;
  stageName: string | undefined;
  editable: boolean;
}>;

/**
 * The ⓘ beside a deal's probability that says why it has that figure. When
 * Margince set it, only the owner asks for the deal's changes to quote its
 * reason. The server writes that reason for the owner alone.
 */
export function ProbabilityHint(props: HintProps) {
  const viewerId = useViewerId();
  return props.deal.win_probability_source === "margince" &&
    props.deal.status === "open" &&
    viewerId !== undefined &&
    props.deal.owner_id === viewerId ? (
    <MarginceProbabilityHint {...props} />
  ) : (
    <ReasonTip {...props} recorded={null} />
  );
}

function MarginceProbabilityHint(props: HintProps) {
  const t = useT();
  const { locale } = useLocale();
  const changes =
    useDealMarginceChanges(props.deal.id).data?.pages.flatMap(
      (page) => page.changes ?? [],
    ) ?? [];
  // Newest first, so the first change that moved the figure set it.
  const setter = changes.find(setsProbability);
  return (
    <ReasonTip
      {...props}
      recorded={setter ? receiptReason(setter, t, locale) : null}
    />
  );
}

function ReasonTip({
  deal,
  stageName,
  editable,
  recorded,
}: HintProps & Readonly<{ recorded: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const reason = probabilityReason(deal, {
    stageName,
    t,
    locale,
    viewerId,
    recorded,
    editable,
  });
  return reason ? (
    <InfoTip label={t("deal.winProbability.why")} text={reason} />
  ) : null;
}
