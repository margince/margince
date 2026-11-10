// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

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
