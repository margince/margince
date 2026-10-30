import type { components } from "../api/schema";
import { acquisitionKindLabel } from "../format/acquisitionkind";
import { COMPANY_LIFECYCLE_LABELS } from "../format/companylifecycle";
import { RELATIONSHIP_TYPE_LABELS } from "../format/companyrelationships";
import { EMAIL_TYPE_LABEL, PHONE_TYPE_LABEL } from "../format/contactchannels";
import { DEAL_STATUS_LABEL } from "../format/dealstatus";
import { formatMoneyOrAbsent, formatNumber } from "../format/format";
import { industryWords } from "../format/industry";
import { LEAD_STATUS_LABEL } from "../format/leadstatus";
import { offerStatusLabel } from "../format/offerstatus";
import { templateLanguage } from "../format/offertemplatelocale";
import { billingModelLabel } from "../format/productbilling";
import { unitLabel } from "../format/productunit";
import { PHASE_LABEL } from "../format/projectphase";
import { promotionTriggerLabel } from "../format/promotion";
import { sendPermissionReasonKey } from "../format/sendpermissionreason";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { ResolvedDisplayField } from "./approvaldisplay.types";
import { lifecyclePreviewFields } from "./approvallifecycle";
import { previewFieldLabel } from "./approvalpreviewfields";
import { importPreviewFields } from "./approvalpreviewimport";
import { mergeValueLabel } from "./approvalpreviewmergevalues";
import { contextOptions } from "./compose-context";
import {
  MEMBER_RETIREMENT_EFFECT_LABELS,
  memberRetirementBlocker,
} from "./memberretirement";
import { KIND_LABELS } from "./relationshipkinds";
import { WON_REASON_LABELS } from "./winreason";

const PREVIEW_EFFECT_LABELS: Record<string, MessageKey> = {
  ...MEMBER_RETIREMENT_EFFECT_LABELS,
  create_activity_record: "approval.import.createActivity",
  import_close_deal: "approval.import.closeDeal",
  rearm_held_message: "approval.scheduled.rearm",
  apply_company_read: "approval.read.effect",
  queue_vat_check: "approval.read.vat_check",
  merge_records_release_source: "approval.preview.mergeRecords",
  fill_empty_lead_fields: "approval.preview.fillLead",
  merge_company_private_readings: "approval.preview.companyPrivateReadings",
  merge_company_preserve_receipts: "approval.preview.companyReceipts",
  merge_company_reconcile_lookups: "approval.preview.companyLookups",
  attach_task_recipients: "approval.preview.attachRecipients",
  archive_relationship_preserve_records: "approval.preview.archiveRelationship",
  archive_relationship_promote_employer:
    "approval.preview.archivePromoteEmployer",
  archive_relationship_no_primary: "approval.preview.archiveNoPrimary",
  merge_tags_release_source: "approval.preview.mergeTags",
  partner_margin_tier_unset: "approval.preview.partnerTierUnset",
  archive_company_detach_scope: "approval.preview.archiveCompany",
  archive_contact_detach_scope: "approval.preview.archiveContact",
  archive_project_detach_scope: "approval.preview.archiveProject",
  archive_deal_detach_scope: "approval.preview.archiveDeal",
  archive_activity_preserve_content: "approval.preview.archiveActivity",
  archive_product_preserve_snapshots: "approval.preview.archiveProduct",
  archive_template_preserve_offers: "approval.preview.archiveTemplate",
  archive_offer_preserve_lines: "approval.preview.archiveOffer",
  record_confirmation_only: "approval.preview.confirmationOnly",
  create_contact_employer: "approval.preview.createEmployer",
  reuse_contact_employer: "approval.preview.reuseEmployer",
  resolve_contact_employer: "approval.preview.resolveEmployer",
  create_contact_record: "approval.preview.createContact",
  record_contact_acquisition: "approval.preview.recordAcquisition",
  create_offer_draft: "approval.preview.createOffer",
  assign_offer_number: "approval.preview.assignOfferNumber",
  create_offer_template: "approval.preview.createTemplate",
  set_template_locale_default: "approval.preview.defaultTemplate",
  create_product_record: "approval.preview.createProduct",
  create_project_record: "approval.preview.createProject",
  assign_project_key: "approval.preview.assignProjectKey",
  create_deal_record: "approval.preview.createDeal",
  create_company_record: "approval.preview.createCompany",
  create_workspace_visible: "approval.preview.workspaceVisible",
  geocode_if_enabled: "approval.preview.geocodeConditional",
  create_lead_record: "approval.preview.createLead",
  reuse_existing_record: "approval.preview.reuseRecord",
  queue_duplicate_review: "approval.preview.queueDuplicate",
  relink_preserve_other_links: "approval.preview.relinkPreserve",
  relink_project_retention: "approval.preview.relinkRetention",
  promote_move_consent_links: "approval.preview.promotionMoveLinks",
  promote_lead_archive_source: "approval.preview.promotionArchive",
  promote_create_contact: "approval.preview.promotionCreate",
  promote_fill_contact: "approval.preview.promotionFill",
  promote_open_deal_seat_contact: "approval.preview.promotionDeal",
  promote_queue_duplicate_review: "approval.preview.promotionReview",
  promote_fill_company_industry: "approval.preview.promotionFillIndustry",
  promote_propose_company: "approval.preview.promotionProposeCompany",
  restore_lead_archive_contact: "approval.preview.demoteArchive",
  restore_lead_preserve_contact: "approval.preview.demotePreserve",
  send_without_human_signature: "approval.preview.unsignedSend",
  append_recipient_unsubscribe: "approval.preview.unsubscribeFooter",
  authorize_channel_reply: "approval.preview.authorizeChannelReply",
  authorize_calendar_invitation: "approval.preview.authorizeInvitation",
  classify_correspondence: "approval.preview.correspondence",
  start_project_delivery: "approval.preview.delivery",
  authorize_message_send: "approval.preview.authorizeMessage",
  archive_lead_remove_tags: "approval.preview.archiveLead",
  record_meeting_with_consent: "approval.preview.recordMeetingConsent",
  preserve_existing_consent: "approval.preview.preserveConsent",
  request_marketing_confirmation: "approval.preview.requestConfirmation",
  marketing_confirmation_unavailable:
    "approval.preview.confirmationUnavailable",
  record_meeting_only: "approval.preview.recordMeeting",
  authorize_website_read: "approval.preview.authorizeWebsite",
};
const ENRICHMENT_DEPTH_LABELS: Record<string, MessageKey> = {
  page: "approval.preview.readPage",
  site: "approval.preview.readSite",
  technical: "approval.preview.readTechnical",
};

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
  switch (value.kind) {
    case "date":
      return formatDay
        ? formatDay(value.text)
        : t("approval.preview.unavailable");
    case "money":
      return previewMoney(value, t, locale);
    case "number": {
      if (
        !["default_tax_rate", "quantity", "discount_pct", "tax_rate"].includes(
          field,
        )
      )
        return previewLabel(value, field, t);
      const rate = Number(value.text);
      return Number.isFinite(rate)
        ? formatNumber(rate, locale)
        : t("approval.preview.unavailable");
    }
    case "structured":
      return t("approval.preview.structured", { value: value.text });
    case "unavailable_name":
      return t("approval.preview.nameUnavailable");
    case "execution":
      return t("approval.preview.execution");
    case "empty":
      if (field === "promotion_restriction_purpose")
        return t("approval.preview.promotionAllMarketing");
      return t(
        field === "communication_context"
          ? "approval.preview.contextAutomatic"
          : "approval.preview.empty",
      );
    case "boolean":
      return t(value.text === "true" ? "field.yes" : "field.no");
    default:
      return previewLabel(value, field, t);
  }
}

const PREVIEW_TEXT_FORMATTERS = new Map<
  string,
  (value: string, t: Translator) => string
>([
  ["creation_acquisition", acquisitionKindLabel],
  ["unit", unitLabel],
  ["billing_model", billingModelLabel],
  ["locale", templateLanguage],
  ["offer_status", offerStatusLabel],
  ["promotion_company_industry", (value, t) => industryWords(t, value)],
]);

const PREVIEW_CATALOG_ALIASES: Readonly<Record<string, string>> = {
  merge_relationship_kind: "relationship_kind",
  merge_participant_role: "relink_participant_role",
  merge_company_relationship_type: "archive_company_type",
  merge_override_authority: "promotion_restriction_authority",
  promotion_consent_before: "consent_result",
  creation_source_consent: "consent_result",
  relationship_end_precision: "relationship_start_precision",
};

function previewLabel(value: Value, field: string, t: Translator): string {
  const mergeLabel = mergeValueLabel(field, value.text, t);
  if (mergeLabel !== undefined) return mergeLabel;
  const formatter = PREVIEW_TEXT_FORMATTERS.get(field);
  if (formatter) return formatter(value.text, t);
  if (field === "promotion_trigger") {
    const label = promotionTriggerLabel(value.text);
    if (label) return t(label);
  }
  const catalogField = PREVIEW_CATALOG_ALIASES[field] ?? field;
  if (field === "promotion_restriction")
    return t(sendPermissionReasonKey(value.text));
  if (field === "promotion_set_origin") return t("field.yes");

  if (field === "communication_context") {
    const option = contextOptions(t).find(
      (option) => option.value === value.text,
    );
    if (option) return option.label;
  }
  const catalogs: Record<string, Readonly<Record<string, MessageKey>>> = {
    relationship_kind: KIND_LABELS,
    archive_company_type: RELATIONSHIP_TYPE_LABELS,
    relationship_start_precision: {
      day: "approval.preview.precisionDay",
      month: "approval.preview.precisionMonth",
    },
    creation_email_type: EMAIL_TYPE_LABEL,
    creation_phone_type: PHONE_TYPE_LABEL,
    relink_participant_role: {
      from: "client.sender",
      to: "compose.to",
      cc: "compose.cc",
      bcc: "compose.bcc",
      attendee: "log.attendee",
      organizer: "approval.preview.meetingHost",
    },
    promotion_basis_kind: {
      contract: "approval.preview.basisContract",
      precontract_request: "approval.preview.basisRequest",
      subject_initiated_correspondence: "approval.preview.basisCorrespondence",
      legitimate_interests: "approval.preview.basisInterests",
      legal_obligation: "approval.preview.basisObligation",
      vital_or_security_interest: "approval.preview.basisSecurity",
      existing_customer_exception: "approval.preview.basisCustomer",
      vn_subject_agreement: "approval.preview.basisAgreement",
    },
    promotion_restriction_authority: {
      machine: "approval.preview.promotionMachine",
      user: "approval.preview.promotionUser",
      admin: "approval.preview.promotionAdmin",
      subject: "approval.preview.promotionSubject",
    },
    won_without_contract_reason: WON_REASON_LABELS,
    lead_status: LEAD_STATUS_LABEL,
    status: DEAL_STATUS_LABEL,
    phase: PHASE_LABEL,
    enrichment_depth: ENRICHMENT_DEPTH_LABELS,
    communication_context: {
      reply_to_inbound: "approval.preview.contextReply",
      security_notice: "approval.preview.contextSecurity",
      privacy_notice: "approval.preview.contextPrivacy",
      record_confirmation: "approval.preview.contextRecord",
      consent_confirmation: "approval.preview.contextConsent",
      optout_confirmation: "approval.preview.contextOptout",
    },
    consent_result: {
      granted: "approval.preview.consentGranted",
      withdrawn: "approval.preview.consentWithdrawn",
    },
  };
  const labels =
    catalogs[catalogField] ??
    (value.kind === "lifecycle" ? COMPANY_LIFECYCLE_LABELS : undefined);
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
const PREVIEW_CONFLICT_LABELS: Record<string, MessageKey> = {
  ...LIFECYCLE_CONFLICT_LABELS,
  sender_signature_private: "approval.preview.signaturePrivate",
  import_scope_unavailable: "approval.preview.importScopeUnavailable",
  email_names_no_one: "approval.preview.machineAddress",
  lead_holds_email: "approval.preview.leadHoldsEmail",
};

export function commercialPreviewDisplay(
  preview: Preview,
  t: Translator,
  locale: Locale = "en",
  formatDay?: (value: string) => string,
  zone?: string,
): readonly ResolvedDisplayField[] {
  const evidence = lifecyclePreviewFields(preview, t, locale, zone);
  const conflicts = (preview.conflicts ?? []).map((conflict, index) => {
    const label = PREVIEW_CONFLICT_LABELS[conflict.kind];
    return {
      field: `preview_conflict_${index}`,
      label: t("approval.preview.conflict"),
      value: label
        ? t(label)
        : conflict.kind.startsWith("member_retirement_")
          ? memberRetirementBlocker(
              conflict.kind.slice("member_retirement_".length),
              t,
            )
          : conflict.summary.trim() || t("approval.preview.incompleteEvidence"),
      comparison: null,
    };
  });
  const readable = preview.version === 1 || preview.version === 2;
  if (!readable || preview.state !== "ready") {
    const key =
      readable && preview.state === "stale"
        ? "approval.preview.stale"
        : readable && preview.state === "incomplete"
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
    value: PREVIEW_EFFECT_LABELS[effect]
      ? t(PREVIEW_EFFECT_LABELS[effect])
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
    ...importPreviewFields(
      preview.import_rows ?? [],
      t,
      (value, field) => previewValue(value, field, t, locale, formatDay),
      (effect) =>
        PREVIEW_EFFECT_LABELS[effect]
          ? t(PREVIEW_EFFECT_LABELS[effect])
          : effect,
    ),
    ...(preview.details ?? []).map((detail, index) => ({
      field: detail.field === "body" ? "body" : `detail_${index}`,
      label: previewFieldLabel(detail.field, t),
      href: detail.value.kind === "link" ? detail.value.text : undefined,
      value: ["body", "html_body", "signature_text", "signature_html"].includes(
        detail.field,
      )
        ? null
        : previewValue(detail.value, detail.field, t, locale, formatDay),
      comparison: null,
    })),
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

// stagedDealMove is a deal move's own arguments. A move staged through the REST
// gate wraps them as {operation, path, body}; a tool call stores them bare.
export function stagedDealMove(
  change: Readonly<Record<string, unknown>> | null | undefined,
): Readonly<Record<string, unknown>> {
  if (!change) return {};
  const body = change.body;
  if (
    typeof change.operation === "string" &&
    typeof body === "object" &&
    body !== null &&
    !Array.isArray(body)
  ) {
    return body as Readonly<Record<string, unknown>>;
  }
  return change;
}

// dealCloseOutcome reads whether a deal move closes the deal. A preview, while
// one exists, decides: its status change is the move's real landing. Once
// decided there is no preview. Then a lost reason in the staged move marks a
// loss, because an agent closing a deal as lost must give one.
export function dealCloseOutcome(
  preview: Preview | null | undefined,
  change?: Readonly<Record<string, unknown>> | null,
): "won" | "lost" | undefined {
  if (preview) {
    const status = preview.changes.find((entry) => entry.field === "status")
      ?.after.text;
    return status === "won" || status === "lost" ? status : undefined;
  }
  const lostReason = stagedDealMove(change).lost_reason;
  return typeof lostReason === "string" && lostReason.trim() !== ""
    ? "lost"
    : undefined;
}
