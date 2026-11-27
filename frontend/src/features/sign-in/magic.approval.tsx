// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Badge, Card } from "../../design-system/atoms";
import { DecisionEvidence } from "../../design-system/decisioncard.content";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { Chip } from "../../design-system/readings";
import { middayInstant } from "../../format/calendarday";
import { formatDate } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { ResolvedDisplayField } from "../../lib/approvaldisplay.types";
import { approvalKindLabel } from "../../lib/approvalkindlabel";
import {
  commercialPreviewDisplay,
  dealCloseOutcome,
} from "../../lib/approvalpreview";
import type { MagicLine } from "../../lib/magic.queries";
import { lineSentence } from "../../lib/magiccopy";
import { useViewerZone } from "../../lib/viewerzone";

type Preview = components["schemas"]["ApprovalPreview"];

const COMMERCIAL_FIELDS = new Set([
  "company_id",
  "owner_id",
  "amount_minor",
  "arr_minor",
  "currency",
]);

/**
 * A decision waiting on the reader, as one of Home's task cards: the staged
 * ring where a task has its tick, what the proposal does, the changes it
 * makes, its chips, and the one press that opens it to be answered.
 */
export function MagicApprovalCard({
  line,
  subject,
  when,
  decide,
}: Readonly<{
  line: MagicLine;
  /** The record the proposal is about, as a chip. */
  subject: ReactNode;
  when: ReactNode;
  decide: ReactNode;
}>) {
  const t = useT();
  const title = approvalTitle(line, t);
  const fields = useApprovalFields(line).filter(
    (field) => field.comparison?.to !== title && field.value !== title,
  );
  const kind = line.approval_kind
    ? approvalKindLabel(line.approval_kind, t)
    : t("magic.approval.reviewChange");
  return (
    <Card as="article" className="brief-task magic-approval">
      <span className="brief-task-tick">
        <span className="magic-mark" data-kind="waiting" aria-hidden="true" />
      </span>
      <div className="magic-approval-body">
        <span className="brief-task-title">{title}</span>
        {fields.length > 0 && (
          <FieldGrid>
            {fields.map((field) => (
              <FieldRow key={field.field} label={field.label}>
                {field.value}
              </FieldRow>
            ))}
          </FieldGrid>
        )}
        {line.approval_preview && (
          <ApprovalBasis preview={line.approval_preview} />
        )}
        <span className="task-meta">
          {line.actor.type === "agent" ? (
            <Badge tone="ai">{kind}</Badge>
          ) : (
            <Chip dense icon={ShieldCheck}>
              {kind}
            </Chip>
          )}
          {subject}
          <span className="task-meta-status">{when}</span>
        </span>
      </div>
      <div className="magic-approval-decide">{decide}</div>
    </Card>
  );
}

// What the proposal does, in its own words: a deal it closes, the task a
// commitment asks for, or the receipt's sentence for every other kind.
function approvalTitle(line: MagicLine, t: Translator): string {
  const preview = line.approval_preview;
  const outcome = preview ? dealCloseOutcome(preview) : undefined;
  if (outcome === "won") return t("magic.approval.markWon");
  if (outcome === "lost") return t("magic.approval.markLost");
  const summary = line.summary.values?.summary;
  if (line.approval_kind === "commitment_task" && summary) return summary;
  return lineSentence(line, t);
}

// The preview's rows a reader weighs before deciding. A stage move shows only
// the fields it changes and the deal's commercial facts.
function useApprovalFields(line: MagicLine): readonly ResolvedDisplayField[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const preview = line.approval_preview;
  if (!preview) return [];
  const display = commercialPreviewDisplay(
    preview,
    t,
    locale,
    (value) => formatDate(middayInstant(value, zone), locale, zone),
    zone,
  );
  const commercialStage = ["advance_deal", "progress_deal"].includes(
    line.approval_kind ?? "",
  );
  return display.filter((field) => {
    if (field.value === null) return false;
    if (!commercialStage) return true;
    if (field.field === "reviewer_reason" || field.field === "preview_status")
      return true;
    if (
      preview.changes.some(
        (change) =>
          change.field === field.field &&
          change.after.kind !== "execution" &&
          !(
            change.field === "status" &&
            preview.changes.some((other) => other.field === "stage_id")
          ),
      )
    )
      return true;
    const index = Number(field.field.replace("detail_", ""));
    const detail = preview.details?.[index];
    return detail !== undefined && COMMERCIAL_FIELDS.has(detail.field);
  });
}

function ApprovalBasis({ preview }: Readonly<{ preview: Preview }>) {
  if (preview.state !== "ready" || preview.version !== 2) return null;
  const hasEvidence = preview.evidence?.some((item) =>
    item.evidence_snippet.trim(),
  );
  return hasEvidence ? (
    <DecisionEvidence evidence={preview.evidence} collapsed={false} />
  ) : null;
}
