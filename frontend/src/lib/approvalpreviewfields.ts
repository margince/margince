import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { MERGE_FIELD_LABELS } from "./approvalpreviewmergefields";
import { historyFieldLabel } from "./historyfieldlabels";

const PREVIEW_DETAIL_LABELS: Record<string, MessageKey> = {
  import_create_tag: "approval.import.createTag",
  import_apply_tag: "approval.import.applyTag",
  import_context_tag: "approval.import.contextTag",
  import_link_company: "approval.import.linkCompany",
  import_link_contact: "approval.import.linkContact",
  import_link_lead: "approval.import.linkLead",
  import_link_deal: "approval.import.linkDeal",
  import_link_row_company: "approval.import.companyRow",
  import_link_row_contact: "approval.import.contactRow",
  import_link_row_lead: "approval.import.leadRow",
  import_link_row_deal: "approval.import.dealRow",
  import_dropped_link: "approval.import.droppedLink",
  retry_delay_minutes: "approval.scheduled.delay",
  read_profile_field: "approval.read.profileField",
  read_fact_field: "approval.read.factField",
  read_value_before: "approval.read.before",
  read_value_after: "approval.read.after",
  read_value_proposed: "approval.read.proposed",
  read_source_url: "approval.read.source",
  read_source_quote: "approval.read.quote",
  read_value_retained: "approval.read.retained",

  stage_required_criterion_met: "approval.preview.requiredCriterionMet",
  stage_required_criterion_unmet: "approval.preview.requiredCriterionUnmet",
  stage_optional_criterion_met: "approval.preview.optionalCriterionMet",
  stage_optional_criterion_unmet: "approval.preview.optionalCriterionUnmet",
  creation_acquisition: "approval.preview.acquisition",
  creation_acquired_at: "approval.preview.acquiredAt",
  creation_source_activity: "approval.preview.sourceActivity",
  creation_source_withheld_activities:
    "approval.preview.sourceWithheldActivities",
  creation_source_consent: "approval.preview.sourceConsent",
  creation_email_type: "field.emailType",
  creation_phone_type: "field.phoneType",
  creation_primary: "field.primary",
  creation_position: "approval.preview.channelPosition",
  creation_email_correspondence: "approval.preview.emailCorrespondence",
  creation_social_platform: "approval.preview.socialPlatform",
  creation_social_handle: "approval.preview.socialHandle",
  converted_from_lead_id: "approval.preview.originLead",
  confirmation_question: "approval.preview.confirmationQuestion",
  confirmation_reason: "approval.preview.confirmationReason",
  offer_number: "deal.offerNumber",
  revision: "deal.offerRevision",
  offer_status: "history.field.status",
  buyer_company_id: "offer.buyerCompany",
  template_id: "offer.template",
  product_id: "search.kind.product",
  valid_until: "offer.validUntil",
  intro_text: "offer.introText",
  terms_text: "offer.termsText",
  net_minor: "offer.net",
  tax_minor: "offer.tax",
  gross_minor: "offer.gross",
  arr_minor: "offer.arr",
  net_tcv_minor: "offer.committedNet",
  offer_line_position: "offer.position",
  quantity: "offer.quantity",
  discount_pct: "offer.discountPct",
  tax_rate: "offer.taxRate",
  interval_count: "offer.committedPeriods",
  line_net_minor: "offer.net",
  line_tax_minor: "offer.tax",
  line_total_minor: "offer.lineTotal",
  template_header: "template.header",
  template_footer: "template.footer",
  locale: "template.locale",
  is_default: "template.isDefault",
  sku: "product.sku",
  unit: "product.unit",
  unit_price_minor: "product.unitPrice",
  default_tax_rate: "product.taxRate",
  active: "product.active",
  billing_model: "product.billingModel",
  billing_interval_months: "approval.preview.billingMonths",
  creation_primary_domain: "approval.preview.primaryDomain",
  address_line1: "create.addressLine1",
  address_line2: "create.addressLine2",
  address_city: "create.city",
  address_region: "create.region",
  address_postal_code: "create.postalCode",
  address_country: "create.country",
  creation_existing: "approval.preview.creationExisting",
  creation_duplicate: "approval.preview.creationDuplicate",
  promotion_withdrawal_address: "approval.preview.withdrawalAddress",
  promotion_link_issued: "approval.preview.linkIssued",
  promotion_link_expires: "approval.preview.linkExpires",
  promotion_record_revoked: "approval.preview.recordRevoked",
  promotion_basis_kind: "approval.preview.basisKind",
  promotion_basis_from: "approval.preview.basisFrom",
  promotion_basis_until: "approval.preview.basisUntil",
  promotion_basis_activity: "approval.preview.basisActivity",
  relink_destination: "approval.preview.relinkDestination",
  relink_activity: "approval.preview.relinkActivity",
  relink_unchanged_activity: "approval.preview.relinkUnchanged",
  relink_removed_link: "approval.preview.relinkRemoved",
  relink_participant_move: "approval.preview.relinkParticipantMove",
  relink_participant_merge: "approval.preview.relinkParticipantMerge",
  relink_participant_role: "approval.preview.relinkRole",
  relink_participant_address: "approval.preview.relinkAddress",
  relink_participant_user: "approval.preview.relinkUser",
  promotion_trigger: "approval.preview.promotionTrigger",
  promotion_note: "approval.preview.promotionNote",
  promotion_evidence: "approval.preview.promotionEvidence",
  promotion_contact_name: "approval.preview.promotionContactName",
  promotion_contact_source: "approval.preview.promotionContactSource",
  promotion_contact_title: "approval.preview.promotionContactTitle",
  promotion_contact_owner: "approval.preview.promotionContactOwner",
  promotion_contact_email: "approval.preview.promotionContactEmail",
  promotion_contact_phone: "approval.preview.promotionContactPhone",
  promotion_existing_contact: "approval.preview.promotionExistingContact",
  promotion_set_origin: "approval.preview.promotionSetOrigin",
  promotion_company_industry: "approval.preview.promotionCompanyIndustry",
  promotion_duplicate_candidate: "approval.preview.promotionDuplicateCandidate",
  promotion_move_activity: "approval.preview.promotionMoveActivity",
  promotion_remove_duplicate_activity_link:
    "approval.preview.promotionDuplicateActivity",
  promotion_move_withheld_activities:
    "approval.preview.promotionWithheldActivities",
  promotion_consent_before: "approval.preview.promotionConsentBefore",
  promotion_restriction: "approval.preview.promotionRestriction",
  promotion_restriction_authority:
    "approval.preview.promotionRestrictionAuthority",
  promotion_restriction_purpose: "approval.preview.promotionRestrictionPurpose",
  promotion_deal_name: "approval.preview.promotionDealName",
  promotion_deal_amount: "approval.preview.promotionDealAmount",
  promotion_deal_owner: "approval.preview.promotionDealOwner",
};

// An extension call's own argument names arrive under this prefix, so none can
// take a detail key this file or approvalevidence.tsx renders as markup.
const EXTENSION_ARGUMENT_PREFIX = "argument:";

export function previewFieldLabel(field: string, t: Translator): string {
  if (field.startsWith(EXTENSION_ARGUMENT_PREFIX)) {
    return field.slice(EXTENSION_ARGUMENT_PREFIX.length);
  }
  const promotion = PREVIEW_DETAIL_LABELS[field] ?? MERGE_FIELD_LABELS[field];
  if (promotion) return t(promotion);
  switch (field) {
    case "existing_task_id":
      return t("approval.preview.existingTask");
    case "recipients":
      return t("worklist.recipients.label");
    case "summary":
      return t("approval.field.step");
    case "consent_contact":
      return t("approval.preview.consentContact");
    case "consent_purpose":
      return t("approval.preview.consentPurpose");
    case "consent_policy_version":
      return t("approval.preview.consentVersion");
    case "consent_wording":
      return t("approval.preview.consentWording");
    case "consent_result":
      return t("approval.preview.consentResult");
    case "marketing_purpose":
      return t("approval.preview.marketingPurpose");
    case "marketing_policy_version":
      return t("approval.preview.marketingVersion");
    case "marketing_wording":
      return t("approval.preview.marketingWording");
    case "confirmation_recipient":
      return t("approval.preview.confirmationRecipient");
    case "invitation_description":
      return t("history.field.description");
    case "meeting_location":
      return t("scheduling.location");
    case "video_call":
      return t("scheduling.videoGeneric");
    case "email_reminder":
      return t("scheduling.emailReminder");
    case "channel_provider":
      return t("approval.preview.channelProvider");
    case "channel_account":
      return t("history.field.channel_identity");
    case "channel_username":
      return t("history.field.channel_username");
    case "communication_context":
      return t("approval.preview.claimedContext");
    case "message_purpose":
      return t("approval.preview.messagePurpose");
    case "operator_reason":
      return t("approval.preview.operatorReason");
    case "contract_id":
      return t("approval.preview.evidenceContract");
    case "invoice_id":
      return t("approval.preview.evidenceInvoice");
    case "deal_id":
      return t("approval.preview.evidenceDeal");
    case "disclosure_text":
      return t("approval.preview.disclosureText");
    case "signature_text":
    case "signature_html":
      return t("approval.preview.senderSignature");
    case "meeting_start":
      return t("approval.preview.meetingStart");
    case "meeting_end":
      return t("approval.preview.meetingEnd");
    case "meeting_host":
      return t("approval.preview.meetingHost");
    case "intended_attendee":
      return t("approval.preview.intendedAttendee");
    case "target_url":
      return t("approval.preview.targetURL");
    case "enrichment_depth":
      return t("approval.preview.readScope");
    case "demotion_reason":
      return t("lead.demoteReason");
    case "demotion_contact":
      return t("approval.preview.demotionContact");
    case "lead_status":
      return t("history.field.status");
    case "disqualify_reason":
      return t("lead.disqualify.reason");
    case "disqualify_note":
      return t("lead.disqualify.note");
    case "to":
      return t("compose.to");
    case "cc":
      return t("compose.cc");
    case "bcc":
      return t("compose.bcc");
    case "relationship_kind":
      return t("rel.kind");
    case "relationship_started":
      return t("rel.startedAt");
    case "relationship_ended":
      return t("rel.endedAt");
    case "relationship_start_precision":
      return t("approval.preview.startPrecision");
    case "relationship_end_precision":
      return t("approval.preview.endPrecision");
    case "contact_id":
      return t("search.kind.contact");
    case "counterparty_contact_id":
    case "counterparty_company_id":
      return t("rel.counterparty");
    case "archive_primary_employer":
      return t("approval.preview.newPrimaryEmployer");
    case "partner_role":
      return t("partner.role");
    case "partner_stage":
      return t("partner.stage");
    case "partner_next_step":
      return t("partner.nextStep");
    case "partner_next_step_due_at":
      return t("partner.nextStepDue");
    case "merge_tag_source":
      return t("approval.preview.mergeTagSource");
    case "merge_tag_target":
      return t("approval.preview.mergeTagTarget");
    case "merge_tag_move":
      return t("approval.preview.mergeTagMove");
    case "merge_tag_collapse":
      return t("approval.preview.mergeTagCollapse");
    case "archive_domain":
      return t("approval.preview.archiveDomain");
    case "archive_company_type":
      return t("approval.preview.archiveCompanyType");
    case "archive_partner":
      return t("approval.preview.archivePartner");
    case "archive_email":
      return t("approval.preview.archiveEmail");
    case "archive_phone":
      return t("approval.preview.archivePhone");
    case "archive_channel":
      return t("approval.preview.archiveChannel");
    case "archive_stakeholder":
      return t("approval.preview.archiveStakeholder");
    case "archive_role":
      return t("approval.preview.archiveRole");
    case "archive_list":
      return t("approval.preview.archiveList");
    case "archive_tag":
      return t("approval.preview.archiveTag");
    case "subject":
      return t("compose.subject");
    case "body":
      return t("compose.body");
    case "html_body":
      return t("approval.preview.htmlBody");
    case "filed_under":
      return t("email.detail.filedUnder");
    case "scheduled_at":
      return t("approval.field.scheduled_at");
    case "scheduled_tz":
      return t("scheduling.timezone");
    case "phase":
      return t("project.phaseLabel");
    case "closed_reason":
      return t("project.advance.reason");
    case "pipeline_id":
      return t("deals.pipeline");
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
