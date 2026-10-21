import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ResolvedDisplayField } from "./approvaldisplay.types";
import { previewFieldLabel } from "./approvalpreviewfields";

type ImportRow = components["schemas"]["ApprovalPreviewImportRow"];
type PreviewValue = components["schemas"]["ApprovalPreviewValue"];

const OUTCOME_LABELS: Record<ImportRow["outcome"], MessageKey> = {
  create: "approval.import.create",
  create_duplicate: "approval.import.duplicate",
  update: "approval.import.update",
  unchanged: "approval.import.unchanged",
  skip: "approval.import.skip",
};

export function importPreviewFields(
  rows: ImportRow[],
  t: Translator,
  format: (value: PreviewValue, field: string) => string,
  effectLabel: (effect: string) => string,
): ResolvedDisplayField[] {
  return rows.flatMap((row, index) => {
    const prefix = `import_${index}`;
    const label = t("approval.import.row", {
      line: String(row.line),
      kind: previewFieldLabel(`${row.object}_id`, t),
      key: row.source_key,
    });
    const fields: ResolvedDisplayField[] = [
      {
        field: prefix,
        label,
        value: t(OUTCOME_LABELS[row.outcome]),
        comparison: null,
      },
    ];
    if (row.target) {
      fields.push({
        field: `${prefix}_target`,
        label,
        value: format(row.target, `${row.object}_id`),
        comparison: null,
      });
    }
    for (const change of row.changes) {
      const from = format(change.before, change.field);
      const to = format(change.after, change.field);
      fields.push({
        field: `${prefix}_${change.field}`,
        label: `${label} · ${previewFieldLabel(change.field, t)}`,
        value: t("approval.preview.change", { before: from, after: to }),
        comparison: { from, to },
      });
    }
    for (const [detailIndex, detail] of row.details.entries()) {
      fields.push({
        field: `${prefix}_detail_${detailIndex}`,
        label: `${label} · ${previewFieldLabel(detail.field, t)}`,
        value: format(detail.value, detail.field),
        comparison: null,
      });
    }
    for (const effect of row.effects) {
      fields.push({
        field: `${prefix}_${effect}`,
        label,
        value: effectLabel(effect),
        comparison: null,
      });
    }
    return fields;
  });
}
