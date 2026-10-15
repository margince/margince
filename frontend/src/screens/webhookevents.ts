import { subscribableEventTypeValues } from "../api/public-events";
import type { MessageKey } from "../i18n/en";

export type SubscribableEventType =
  (typeof subscribableEventTypeValues)[number];

export type WebhookEventGroup = Readonly<{
  key: string;
  types: readonly SubscribableEventType[];
}>;

// The group each event type's prefix belongs to, in the order a reader looks
// for them. The types come from the generated catalog, so this names prefixes
// only; webhookevents.test.ts fails when a new prefix has no group.
const PREFIX_GROUPS: ReadonlyArray<readonly [string, readonly string[]]> = [
  ["deals", ["deal"]],
  ["offers", ["offer"]],
  ["pipelines_stages", ["pipeline", "stage"]],
  ["contacts", ["contact", "conversation_claim", "relationship_nudge"]],
  ["companies", ["company"]],
  ["leads", ["lead", "lead_source", "lead_disqualify_reason"]],
  ["activities_email", ["activity", "engagement", "comms"]],
  ["files", ["attachment"]],
  ["projects", ["project"]],
  ["contracts", ["contract"]],
  ["commissions", ["commission"]],
  ["lists", ["list"]],
  ["deal_rooms", ["deal_room"]],
  ["introductions", ["intro_request"]],
  ["consent_privacy", ["consent", "retention"]],
  ["signals", ["signal"]],
  ["forecasts_weekly_plans", ["forecast", "weekly_plan"]],
  ["notices_notifications", ["notice", "notification", "user_delivery"]],
  ["members_access", ["user", "role", "team", "passport", "onboarding"]],
  [
    "member_settings_linkedin",
    [
      "user_display_name",
      "user_greeting_name",
      "user_locale",
      "email_signature",
      "linkedin_account",
      "linkedin_network",
      "linkedin_match",
    ],
  ],
  ["voice_dna", ["voice"]],
  ["approvals_audit", ["approval", "coldstart", "audit"]],
];

export const WEBHOOK_EVENT_GROUPS: readonly WebhookEventGroup[] =
  PREFIX_GROUPS.map(([key, prefixes]) => ({
    key,
    types: subscribableEventTypeValues.filter((type) =>
      prefixes.includes(type.slice(0, type.indexOf("."))),
    ),
  }));

export function eventGroupLabelKey(group: string): MessageKey {
  return `webhooks.eventGroup.${group}` as MessageKey;
}

export function eventLabelKey(type: string): MessageKey {
  return `webhooks.event.${type}.label` as MessageKey;
}

export function eventWhenKey(type: string): MessageKey {
  return `webhooks.event.${type}.when` as MessageKey;
}
