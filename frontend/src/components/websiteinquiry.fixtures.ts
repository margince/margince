// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  WEBSITE_INQUIRY_RECIPE,
  type WebsiteInquiryPolicy,
} from "../lib/websiteinquiries";
import type { GrantSpec } from "../testing/mefixture";

export const INQUIRY_GRANTS: GrantSpec = {
  integrations: ["read", "update"],
  automation: ["read", "create", "update"],
};
export const INQUIRY_AUTHORIZER = "00000000-0000-4000-8000-000000000001";
export const INQUIRY_COORDINATOR = "00000000-0000-4000-8000-000000000002";
export const INQUIRY_ID = "00000000-0000-4000-8000-000000000003";
export const INQUIRY_FORM: components["schemas"]["WebForm"] = {
  id: "00000000-0000-4000-8000-000000000004",
  name: "Website contact",
  form_key: "website-inquiry-example-key",
  sender: "browser",
  allowed_origins: ["https://example.test"],
  lead_origin: "website",
  owner_id: null,
  field_map: {},
  consent_required: false,
  state: "active",
  secret_configured: false,
  version: 4,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  last_post_at: null,
  handling_automation_id: INQUIRY_ID,
};
export const INQUIRY_POLICY: WebsiteInquiryPolicy = {
  form_id: INQUIRY_FORM.id,
  mode: "coordinator",
  routing: {},
  coordinator_id: INQUIRY_COORDINATOR,
  task_title: "Answer the website inquiry",
  due_in_days: 1,
  timezone: "Europe/Berlin",
};
export const INQUIRY_AUTOMATION: components["schemas"]["Automation"] = {
  id: INQUIRY_ID,
  name: "Website inquiries",
  key: WEBSITE_INQUIRY_RECIPE,
  status: "paused",
  owner_id: INQUIRY_AUTHORIZER,
  params: { ...INQUIRY_POLICY },
  inquiry_policy: INQUIRY_POLICY,
  version: 7,
  created_at: "2026-09-01T09:00:00Z",
};
export const INQUIRY_MEMBERS = [
  {
    id: INQUIRY_AUTHORIZER,
    display_name: "Test User",
    email: "authorizer@example.test",
    status: "active",
    is_agent: false,
    can_own_records: true,
  },
  {
    id: INQUIRY_COORDINATOR,
    display_name: "Mina Coordinator",
    email: "coordinator@example.test",
    status: "active",
    is_agent: false,
    can_own_records: true,
  },
];

export const INQUIRY_REQUEST_FIELD: components["schemas"]["CustomField"] = {
  id: "00000000-0000-4000-8000-000000000005",
  object: "lead",
  label: "Request type",
  slug: "request_type",
  type: "picklist",
  status: "active",
  column_name: "cf_request_type",
  show_in_create: true,
  reportable: false,
  created_by: INQUIRY_AUTHORIZER,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  options: ["Quote", "Support"],
};

export const INQUIRY_DEADLINE_PREVIEW: components["schemas"]["WebsiteInquiryHandlingPreview"] =
  {
    received_at: "2026-03-28T15:00:00Z",
    example_due_at: "2026-03-29T21:59:59.999999Z",
    items: [],
  };
