import type { components } from "../../api/schema";
import { formatMoneyOrAbsent } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import { historyFieldLabel } from "../../lib/historyfieldlabels";
import { DEAL_STATUS_LABEL } from "../deals/dealdials";
import { WON_REASON_LABELS } from "../deals/winreason";
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

export function commercialPreviewDisplay(
  preview: Preview,
  t: Translator,
  locale: Locale = "en",
  formatDay?: (value: string) => string,
): readonly ResolvedDisplayField[] {
  const conflicts = (preview.conflicts ?? []).map((conflict, index) => ({
    field: `preview_conflict_${index}`,
    label: t("approval.preview.conflict"),
    value: conflict.summary,
    comparison: null,
  }));
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
    return {
      field: change.field,
      label: previewFieldLabel(change.field, t),
      value: t("approval.preview.change", { before: from, after: to }),
      comparison: { from, to },
    };
  });
  return [
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
