// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Panel } from "../../design-system/panel";
import { formatAnalyticsNumber } from "../../format/format";
import { useLocale } from "../../i18n";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";

// The doors to the records the outlook names, in the order its prose names
// them.
export function OverviewActions({
  evaluation,
  order,
  onEvidence,
  framed = false,
}: Readonly<{
  evaluation: ReportingEvaluation;
  order?: readonly string[];
  onEvidence: (reference: ReportingEvidenceRef) => void;
  // On a card of its own, when no outlook panel holds the doors.
  framed?: boolean;
}>) {
  const { locale } = useLocale();
  const candidates = evaluation.overview_actions ?? [];
  const actions = order?.length
    ? [...candidates].sort((a, b) => {
        const rank = (id: string) => {
          const at = order.indexOf(id);
          return at < 0 ? order.length : at;
        };
        return rank(a.id) - rank(b.id);
      })
    : candidates;
  if (!actions.length) return null;
  const doors = actions.map((action) => (
    <Button
      key={action.id}
      variant="ghost"
      onClick={() => onEvidence(action.evidence)}
    >
      {action.label} ({formatAnalyticsNumber(action.count, locale)})
    </Button>
  ));
  return framed ? (
    <Panel actions={doors}>{null}</Panel>
  ) : (
    <div className="card-actions">{doors}</div>
  );
}
