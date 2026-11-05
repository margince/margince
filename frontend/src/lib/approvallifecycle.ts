import type { components } from "../api/schema";
import { activityDay } from "../format/activitywhen";
import type { Locale, Translator } from "../i18n";
import type { ResolvedDisplayField } from "./approvaldisplay.types";

// The account ending a lifecycle proposal rests on: the scope its source names
// and the date it was said, read on the record's clock (`zone`).

type Preview = components["schemas"]["ApprovalPreview"];

export function lifecyclePreviewFields(
  preview: Preview,
  t: Translator,
  locale: Locale,
  zone?: string,
): ResolvedDisplayField[] {
  const evidence = preview.lifecycle_evidence;
  if (!evidence) return [];
  const labels = {
    company: "approval.preview.scopeCompany",
    workstream: "approval.preview.scopeWorkstream",
    unknown: "approval.preview.scopeUnknown",
  } satisfies Record<typeof evidence.scope, Parameters<Translator>[0]>;
  const fields: ResolvedDisplayField[] = [
    {
      field: "lifecycle_scope",
      label: t("approval.preview.scope"),
      value: t(labels[evidence.scope] ?? "approval.preview.scopeUnknown"),
      comparison: null,
    },
    {
      field: "source_date",
      label: t("approval.preview.sourceDate"),
      value:
        zone === undefined
          ? t("approval.preview.unavailable")
          : activityDay(
              evidence.occurred_at,
              evidence.occurred_precision,
              locale,
              zone,
            ),
      comparison: null,
    },
  ];
  if (evidence.workstream)
    fields.push({
      field: "workstream",
      label: t("approval.preview.workstream"),
      value: evidence.workstream,
      comparison: null,
    });
  return fields;
}
