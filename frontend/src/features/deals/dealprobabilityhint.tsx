// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { InfoTip } from "../../design-system/infotip";
import { useLocale, useT } from "../../i18n";
import { useDealMarginceChanges } from "../../lib/marginchanges";
import { receiptReason } from "../../lib/receiptcopy";
import { useViewerId } from "../../lib/session";
import { probabilityReason } from "./dealprobability";
import type { Deal } from "./deals";

// The audit column a Margince probability change writes, as its receipt
// names the field.
const PROBABILITY_FIELD = "win_probability_override";

type HintProps = Readonly<{ deal: Deal; stageName: string | undefined }>;

/**
 * The ⓘ beside a deal's probability that says why it has that figure. Only a
 * figure Margince set asks for the deal's changes, to quote its own reason.
 */
export function ProbabilityHint(props: HintProps) {
  return props.deal.win_probability_source === "margince" &&
    props.deal.status === "open" ? (
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
  const setter = changes.find((change) =>
    change.changes?.some((moved) => moved.field === PROBABILITY_FIELD),
  );
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
  recorded,
}: HintProps & Readonly<{ recorded: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const reason = probabilityReason(
    deal,
    stageName,
    t,
    locale,
    viewerId,
    recorded,
  );
  return reason ? (
    <InfoTip label={t("deal.winProbability.why")} text={reason} />
  ) : null;
}
