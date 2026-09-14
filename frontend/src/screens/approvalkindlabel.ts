import type { MessageKey } from "../i18n/en";

// What a staged proposal is, in words a reader recognises.
//
// `approval.kind` is a wire enum — `site_lead`, `fx_rate_proposal` — and it was
// rendered verbatim wherever a proposal was listed. A reader deciding whether
// to accept twenty-five of something needs to know what that something is, and
// snake_case in the German UI is not a translation of anything.
//
// The set is the approvals module's grant maps, and this map is pinned against
// them by backend/gates/frontendapprovalkinds_test.go, which DERIVES the corpus
// rather than restating it. The gate this replaced compared against a list
// hand-copied into the frontend's own test, and a mirror of a mirror agrees
// with itself: eleven stageable kinds had no label and two labels named kinds
// the server had already dropped.
//
// What holds a kind to those maps is the compose-side census over every
// production staging site, not the staging writer itself — that one inserts
// the kind it is handed. So this map covers what the product stages, and the
// fallback below is what a kind reaching a reader some other way gets.
//
// A kind that still slips through falls back to its own words rather than its
// identifier: it must degrade to "site lead", never to a token that only makes
// sense to whoever wrote the server.

export const KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  advance_deal: "approval.kind.advance_deal",
  progress_deal: "approval.kind.advance_deal",
  promote_lead: "approval.kind.promote_lead",
  archive_record: "approval.kind.archive_record",
  merge_records: "approval.kind.merge_records",
  // Not a second spelling of merge_records: that folds one RECORD into another
  // and keeps a pointer home, this folds a vocabulary WORD and releases its
  // name for good. The label says which word survives, because that is the
  // half a reader has to check before releasing it.
  merge_tags: "approval.kind.merge_tags",
  update_record: "approval.kind.update_record",
  create_record: "approval.kind.create_record",
  send_email: "approval.kind.send_email",
  // Named for what the reader has to DO, not for what produced it: the row is
  // an email waiting to be read and released, and "held draft" describes its
  // state in a queue rather than the decision in front of them.
  held_draft: "approval.kind.held_draft",
  book_meeting: "approval.kind.book_meeting",
  coldstart: "approval.kind.coldstart",
  // Not a change to a record — a question about a credential's volume, which is
  // why its label says what a yes DOES rather than naming an object.
  volume_release: "approval.kind.volume_release",
  enrich: "approval.kind.enrich",
  deepread: "approval.kind.deepread",
  linkedin_match: "approval.kind.linkedin_match",
  site_lead: "approval.kind.site_lead",
  close_date_correction: "approval.kind.close_date_correction",
  deal_follow_up: "approval.kind.deal_follow_up",
  capture_counterparty: "approval.kind.capture_counterparty",
  company_name_promotion: "approval.kind.company_name_promotion",
  vcard_create: "approval.kind.vcard_create",
  lifecycle_change: "approval.kind.lifecycle_change",
  transcript_proposal: "approval.kind.transcript_proposal",
  stage_progression: "approval.kind.stage_progression",
  fx_rate_proposal: "approval.kind.fx_rate_proposal",
  ai_model_rate_proposal: "approval.kind.ai_model_rate_proposal",
  disqualify_lead: "approval.kind.disqualify_lead",
  demote_lead: "approval.kind.demote_lead",
  advance_project_phase: "approval.kind.advance_project_phase",
  assign_owner: "approval.kind.assign_owner",
  communication_review: "approval.kind.communication_review",
  commit_import: "approval.kind.commit_import",
  emit_flow_event: "approval.kind.emit_flow_event",
  relink_activity: "approval.kind.relink_activity",
  relink_thread: "approval.kind.relink_thread",
  relink_activities: "approval.kind.relink_activities",
  // Distinct from held_draft above, and not a second spelling of it: that one
  // is a reply automation COMPOSED and is waiting to be sent, this one is a
  // message that was already scheduled and got stopped. Different lifecycles,
  // so different words.
  scheduled_send_held: "approval.kind.scheduled_send_held",
  send_account_email: "approval.kind.send_account_email",
  send_message: "approval.kind.send_message",
};
