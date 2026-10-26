import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { DecisionEvidence } from "../../design-system/decisioncard.content";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { Stack } from "../../design-system/stack";
import { middayInstant } from "../../format/calendarday";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  commercialPreviewDisplay,
  dealCloseOutcome,
} from "../../lib/approvalpreview";
import { useRecordZone } from "../../lib/recordzone";
import type { MagicLine } from "./magic.queries";

type Preview = components["schemas"]["ApprovalPreview"];

const COMMERCIAL_FIELDS = new Set([
  "company_id",
  "owner_id",
  "amount_minor",
  "arr_minor",
  "currency",
]);

export function MagicApproval({
  line,
  subject,
  action,
}: Readonly<{ line: MagicLine; subject: ReactNode; action: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const preview = line.approval_preview;
  if (!preview) return null;
  const display = commercialPreviewDisplay(
    preview,
    t,
    locale,
    (value) => formatDate(middayInstant(value, zone), locale, zone),
    zone,
  );
  const commercialStage = [
    "advance_deal",
    "progress_deal",
    "stage_progression",
  ].includes(line.approval_kind ?? "");
  const fields = display.filter((field) => {
    if (!commercialStage) return field.value !== null;
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
  const outcome = dealCloseOutcome(preview);
  const title =
    outcome === "won"
      ? "magic.approval.markWon"
      : outcome === "lost"
        ? "magic.approval.markLost"
        : "magic.approval.reviewChange";
  return (
    <Stack>
      <p>
        <strong>
          {title === "magic.approval.reviewChange" ? action : t(title)}
        </strong>{" "}
        {subject}
      </p>
      <FieldGrid>
        {fields.map(
          (field) =>
            field.value !== null && (
              <FieldRow key={field.field} label={field.label}>
                {field.value}
              </FieldRow>
            ),
        )}
      </FieldGrid>
      <ApprovalBasis preview={preview} />
    </Stack>
  );
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
