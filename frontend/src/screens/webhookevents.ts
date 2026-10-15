import type { subscribableEventTypeValues } from "../api/public-events";
import type { MessageKey } from "../i18n/en";

export type SubscribableEventType =
  (typeof subscribableEventTypeValues)[number];

// The subscribable catalog grouped by the record each event is about, in the
// order a reader looks for them. webhookevents.test.ts holds that every
// subscribable type sits in one group and no other.
export type WebhookEventGroup = Readonly<{
  key: string;
  types: readonly SubscribableEventType[];
}>;

export const WEBHOOK_EVENT_GROUPS: readonly WebhookEventGroup[] = [
  {
    key: "deals",
    types: [
      "deal.created",
      "deal.updated",
      "deal.stage_changed",
      "deal.owner_changed",
      "deal.archived",
      "deal.restored",
    ],
  },
  {
    key: "offers",
    types: [
      "offer.created",
      "offer.sent",
      "offer.accepted",
      "offer.rejected",
      "offer.superseded",
    ],
  },
  {
    key: "pipelines_stages",
    types: [
      "pipeline.created",
      "pipeline.updated",
      "pipeline.archived",
      "stage.created",
      "stage.updated",
      "stage.archived",
    ],
  },
  {
    key: "contacts",
    types: [
      "contact.created",
      "contact.updated",
      "contact.merged",
      "contact.archived",
      "contact.restored",
      "conversation_claim.captured",
      "conversation_claim.changed",
      "relationship_nudge.decided",
    ],
  },
  {
    key: "companies",
    types: [
      "company.created",
      "company.updated",
      "company.merged",
      "company.archived",
      "company.restored",
    ],
  },
  {
    key: "leads",
    types: [
      "lead.created",
      "lead.updated",
      "lead.disqualified",
      "lead.promoted",
      "lead.demoted",
      "lead.merged",
      "lead.sla_breached",
      "lead_source.changed",
      "lead_disqualify_reason.changed",
    ],
  },
  {
    key: "activities_email",
    types: [
      "activity.captured",
      "activity.updated",
      "activity.archived",
      "activity.disposition_recorded",
      "activity.marked_handled",
      "engagement.reply",
      "comms.delivery_bounced",
    ],
  },
  {
    key: "files",
    types: ["attachment.created", "attachment.updated", "attachment.archived"],
  },
  {
    key: "projects",
    types: [
      "project.created",
      "project.updated",
      "project.phase_changed",
      "project.archived",
    ],
  },
  {
    key: "contracts",
    types: [
      "contract.created",
      "contract.updated",
      "contract.status_changed",
      "contract.archived",
    ],
  },
  {
    key: "commissions",
    types: ["commission.accrued", "commission.decided"],
  },
  {
    key: "lists",
    types: [
      "list.created",
      "list.updated",
      "list.archived",
      "list.restored",
      "list.member_added",
      "list.member_removed",
      "list.evaluated",
    ],
  },
  {
    key: "deal_rooms",
    types: [
      "deal_room.opened",
      "deal_room.updated",
      "deal_room.paused",
      "deal_room.resumed",
      "deal_room.closed",
      "deal_room.archived",
      "deal_room.participant_invited",
      "deal_room.participant_revoked",
      "deal_room.participant_credential_reissued",
      "deal_room.comment_posted",
      "deal_room.thread_resolved",
      "deal_room.decision_recorded",
    ],
  },
  {
    key: "introductions",
    types: [
      "intro_request.created",
      "intro_request.decided",
      "intro_request.completed",
      "intro_request.replied",
      "intro_request.closed",
    ],
  },
  {
    key: "consent_privacy",
    types: [
      "consent.changed",
      "consent.suppressed",
      "consent.suppression_lifted",
      "consent.override_recorded",
      "consent.override_lifted",
      "retention.applied",
      "retention.restricted",
    ],
  },
  {
    key: "signals",
    types: ["signal.detected", "signal.resolved"],
  },
  {
    key: "forecasts_weekly_plans",
    types: [
      "forecast.created",
      "forecast.assurance_created",
      "forecast.exception_resolved",
      "forecast.snapshot_created",
      "forecast.share_issued",
      "forecast.share_revoked",
      "weekly_plan.updated",
      "weekly_plan.help_requested",
    ],
  },
  {
    key: "notices_notifications",
    types: [
      "notice.created",
      "notice.read",
      "notification.preference_changed",
      "user_delivery.changed",
    ],
  },
  {
    key: "members_access",
    types: [
      "user.invited",
      "user.activated",
      "user.password_link_issued",
      "user.deactivated",
      "user.reactivated",
      "role.changed",
      "team.changed",
      "passport.revoked",
      "onboarding.state_changed",
    ],
  },
  {
    key: "member_settings_linkedin",
    types: [
      "user_display_name.changed",
      "user_greeting_name.changed",
      "user_locale.changed",
      "email_signature.changed",
      "linkedin_account.changed",
      "linkedin_network.imported",
      "linkedin_match.decided",
    ],
  },
  {
    key: "voice_dna",
    types: [
      "voice.profile_created",
      "voice.profile_updated",
      "voice.profile_archived",
      "voice.corpus_changed",
      "voice.build_changed",
      "voice.version_changed",
      "voice.draft_outcome_recorded",
    ],
  },
  {
    key: "approvals_audit",
    types: [
      "approval.requested",
      "approval.decided",
      "approval.failure_acknowledged",
      "coldstart.read_back_proposed",
      "coldstart.accepted",
      "coldstart.rejected",
      "audit.appended",
    ],
  },
];

export function eventGroupLabelKey(group: string): MessageKey {
  return `webhooks.eventGroup.${group}` as MessageKey;
}

export function eventLabelKey(type: string): MessageKey {
  return `webhooks.event.${type}.label` as MessageKey;
}

export function eventWhenKey(type: string): MessageKey {
  return `webhooks.event.${type}.when` as MessageKey;
}
