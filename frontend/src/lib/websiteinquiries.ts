// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export type WebsiteInquiryPolicy =
  components["schemas"]["WebsiteInquiryPolicy"];
export type WebsiteInquiryMode = components["schemas"]["WebsiteInquiryMode"];
export const WEBSITE_INQUIRY_RECIPE = "website_inquiry";
export const WEB_FORMS_KEY = "web-forms";

export function newInquiryPolicy(
  title: string,
  formId = "",
): WebsiteInquiryPolicy {
  return {
    form_id: formId,
    mode: "one_colleague",
    routing: { owners: [] },
    coordinator_id: "",
    task_title: title,
    due_in_days: 1,
    timezone: "",
  };
}

export function inquiryReasonKey(reason: string | undefined): MessageKey {
  switch (reason) {
    case "existing_owner":
      return "inquiry.reason.owner";
    case "owner_needs_review":
      return "inquiry.reason.ownerReview";
    case "recipient_no_longer_eligible":
      return "inquiry.reason.recipientChanged";
    case "coordinator_review":
      return "inquiry.reason.coordinator";
    case "round_robin":
      return "inquiry.reason.rotation";
    case "inquiry_task_exists":
      return "inquiry.result.alreadyHandled";
    case undefined:
      return "inquiry.reason.notRecorded";
    default:
      return "inquiry.reason.selected";
  }
}

export function isWebsiteInquiryTask(
  task: Pick<components["schemas"]["Activity"], "source_system" | "source_id">,
): boolean {
  return (
    Boolean(task.source_id) &&
    (task.source_system === WEBSITE_INQUIRY_RECIPE ||
      (task.source_system === "lead_for_contact" &&
        task.source_id?.startsWith("web_form:") === true))
  );
}
