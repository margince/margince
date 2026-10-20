import type { components } from "../../api/schema";
import { DEAL_STATUS_LABEL } from "../../components/dealdials";
import { formatMoneyOrAbsent } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import { historyFieldLabel } from "../../lib/historyfieldlabels";
import { WON_REASON_LABELS } from "../../lib/winreason";
import type { ResolvedDisplayField } from "./approvaldisplay.types";
import { COMPANY_LIFECYCLE_LABELS } from "./approvalkind";

type Preview = components["schemas"]["ApprovalPreview"];
type Value = components["schemas"]["ApprovalPreviewValue"];

function previewMoney(value: Value, t: Translator, locale: Locale): string {
  const amount = Number(value.text);
  return Number.isSafeInteger(amount)
    ? formatMoneyOrAbsent(amount, value.currency, locale)
    : t("approval.preview.minorUnits", {
        amount: value.text,
        currency: value.currency ?? "",
      });
}

function previewValue(
  value: Value,
  field: string,
  t: Translator,
  locale: Locale,
  formatDay?: (value: string) => string,
): string {
  if (value.kind === "date")
    return formatDay
      ? formatDay(value.text)
      : t("approval.preview.unavailable");
  if (value.kind === "money") return previewMoney(value, t, locale);
  if (value.kind === "structured")
    return t("approval.preview.structured", { value: value.text });
  if (value.kind === "unavailable_name")
    return t("approval.preview.nameUnavailable");
  if (value.kind === "execution") return t("approval.preview.execution");
  if (value.kind === "empty") return t("approval.preview.empty");
  if (value.kind === "boolean")
    return t(value.text === "true" ? "field.yes" : "field.no");
  return previewLabel(value, field, t);
}

function previewLabel(value: Value, field: string, t: Translator): string {
  const labels =
    field === "won_without_contract_reason"
      ? WON_REASON_LABELS
      : field === "status"
        ? DEAL_STATUS_LABEL
        : value.kind === "lifecycle"
          ? COMPANY_LIFECYCLE_LABELS
          : undefined;
  const label = Object.entries(labels ?? {}).find(
    ([code]) => code === value.text,
  )?.[1];
  if (label) return t(label);

  return value.text;
}

export const LIFECYCLE_CONFLICT_LABELS: Record<
  string,
  Parameters<Translator>[0]
> = {
  active_contract: "approval.preview.activeContract",
  active_delivery: "approval.preview.activeDelivery",
  unknown_scope: "approval.preview.unknownScope",
  incomplete_evidence: "approval.preview.incompleteEvidence",
  newer_evidence: "approval.preview.newerEvidence",
};

export function commercialPreviewDisplay(
  preview: Preview,
  t: Translator,
  locale: Locale = "en",
  formatDay?: (value: string) => string,
): readonly ResolvedDisplayField[] {
  const evidence = lifecyclePreviewFields(preview, t, formatDay);
  const conflicts = (preview.conflicts ?? []).map((conflict, index) => {
    const label = LIFECYCLE_CONFLICT_LABELS[conflict.kind];
    return {
      field: `preview_conflict_${index}`,
      label: t("approval.preview.conflict"),
      value: label
        ? t(label)
        : conflict.summary.trim() || t("approval.preview.incompleteEvidence"),
      comparison: null,
    };
  });
  if (preview.version !== 1 || preview.state !== "ready") {
    const key =
      preview.version === 1 && preview.state === "stale"
        ? "approval.preview.stale"
        : preview.version === 1 && preview.state === "incomplete"
          ? "approval.preview.incomplete"
          : "approval.preview.unavailable";
    return [
      {
        field: "preview_status",
        label: t("approval.preview.status"),
        value: t(key),
        comparison: null,
        lead: true,
      },
      ...evidence,
      ...conflicts,
    ];
  }
  const effects = (preview.effects ?? []).map((effect) => ({
    field: `effect_${effect}`,
    label: t("approval.preview.effects"),
    value:
      effect === "classify_correspondence"
        ? t("approval.preview.correspondence")
        : effect === "start_project_delivery"
          ? t("approval.preview.delivery")
          : effect === "attach_task_recipients"
            ? t("approval.preview.attachRecipients")
            : effect,
    comparison: null,
  }));
  const changes = preview.changes.map((change) => {
    const from = previewValue(
      change.before,
      change.field,
      t,
      locale,
      formatDay,
    );
    const to = previewValue(change.after, change.field, t, locale, formatDay);
    const sameReference =
      change.before.kind === "reference" &&
      change.after.kind === "reference" &&
      change.before.entity_id !== undefined &&
      change.before.entity_id === change.after.entity_id &&
      change.before.entity_type === change.after.entity_type;
    return {
      field: change.field,
      label: previewFieldLabel(change.field, t),
      value: sameReference
        ? to
        : t("approval.preview.change", { before: from, after: to }),
      comparison: sameReference ? null : { from, to },
    };
  });
  return [
    ...evidence,
    ...conflicts,
    ...changes,
    ...effects,
    {
      field: "reviewer_reason",
      label: t("approval.preview.reviewer"),
      value: reviewAuthority(preview.reviewer_reason, t),
      comparison: null,
    },
  ];
}

function reviewAuthority(reason: string, t: Translator): string {
  const key =
    reason === "requested_decider"
      ? "approval.preview.requested"
      : reason === "on_behalf_of"
        ? "approval.preview.onBehalf"
        : reason === "record_authority"
          ? "approval.preview.authority"
          : "approval.preview.authorityUnknown";
  return t(key);
}

export function editedCommercialPreview(
  preview: Preview | undefined,
  draft: Readonly<Record<string, string>>,
  editing = true,
): Preview | undefined {
  if (!preview || !editing) return preview;
  return {
    ...preview,
    changes: preview.changes.map((change) => {
      const key =
        change.field === "lifecycle" ? "proposed_lifecycle" : change.field;
      const text = draft[key];
      if (
        !Object.hasOwn(draft, key) ||
        text === undefined ||
        !["text", "lifecycle", "date"].includes(change.after.kind)
      )
        return change;
      return { ...change, after: { ...change.after, text } };
    }),
  };
}

function previewFieldLabel(field: string, t: Translator): string {
  switch (field) {
    case "existing_task_id":
      return t("approval.preview.existingTask");
    case "recipients":
      return t("worklist.recipients.label");
    case "summary":
      return t("approval.field.step");
    case "due_date":
      return t("approval.field.due_date");
    case "stage_id":
      return t("deals.stage");
    case "won_without_contract_reason":
      return t("deals.winReason");
    case "won_without_contract_detail":
      return t("deals.winReasonDetail");
    case "closed_at":
      return t("history.field.closed_at");
    case "amount_minor_base":
      return t("history.field.amount_minor_base");
    default:
      return historyFieldLabel(field, t);
  }
}

function lifecyclePreviewFields(
  preview: Preview,
  t: Translator,
  formatDay?: (value: string) => string,
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
      value: formatDay?.(evidence.occurred_at) ?? evidence.occurred_at,
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
