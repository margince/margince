// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { GrantSpec } from "../app/mefixture";
import { PrivacyLanes } from "./settings.privacy";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The whole Privacy and retention page, every lane populated, in the order an
// officer works them.

const PAGE = { next_cursor: null, has_more: false };
const page = (data: unknown[]) => jsonResponse({ data, page: PAGE });

const CONTACT = "00000000-0000-4000-8000-0000000000c1";

const ROUTES: RouteMap = {
  "GET /consent-purposes": () =>
    page([
      {
        id: "p1",
        key: "marketing_email",
        label: "Marketing email",
        requires_double_opt_in: true,
        created_at: "2026-01-01T00:00:00Z",
      },
      {
        id: "p2",
        key: "transactional",
        label: "Transactional email",
        requires_double_opt_in: false,
        created_at: "2026-01-01T00:00:00Z",
      },
    ]),
  "GET /retention/settings": () => jsonResponse({ retain_only: false }),
  "GET /retention-policies": () =>
    page([
      {
        id: "00000000-0000-4000-8000-0000000000a1",
        scope: "activity/transcript",
        object_type: "activity",
        category: "transcript",
        retain_days: 365,
        action: "erase",
        lawful_basis: "Art. 9(2)(a)",
        enabled: true,
        suppressed_by_posture: false,
      },
    ]),
  "GET /retention/restrictions": () =>
    page([
      {
        activity_id: "00000000-0000-4000-8000-0000000000b1",
        kind: "email",
        occurred_at: "2025-03-04T09:00:00Z",
        restricted_at: "2026-08-18T07:00:00Z",
        restricted_until: "2032-01-01T00:00:00Z",
        reason: "commercial_correspondence · §257 HGB / §147 AO",
        deals: [
          { id: "00000000-0000-4000-8000-0000000000d1", name: "Acme rollout" },
        ],
        redacted_fields: ["raw"],
      },
    ]),
  "GET /data-subject-requests": () =>
    page([
      {
        id: "d1",
        kind: "erasure",
        subject_ref: CONTACT,
        subject_label: "Lena Hoffmann",
        status: "in_progress",
        assignee_id: "u-1",
        due_at: "2026-08-01T00:00:00Z",
        created_at: "2026-07-01T00:00:00Z",
      },
      {
        id: "d2",
        kind: "access",
        subject_ref: "partner-reference-0042@acme.test",
        subject_label: null,
        status: "open",
        due_at: "2099-10-01T00:00:00Z",
        created_at: "2026-08-01T00:00:00Z",
      },
    ]),
  "GET /confirm-submissions": () =>
    page([
      {
        id: "01a05500-0000-7000-8000-0000000000c1",
        contact_id: CONTACT,
        kind: "correction",
        field: "full_name",
        current_value: "Schmitt",
        proposed_value: "Schmidt",
        contact_name: "Anna Schmidt",
        submitted_at: "2026-08-01T09:00:00Z",
      },
    ]),
  "GET /privacy/notice-cases": () =>
    page([
      {
        id: "nc-1",
        contact_id: CONTACT,
        contact_name: "Lena Hoffmann",
        acquisition: {
          kind: "purchased_or_imported",
          occurred_at: "2026-01-03T09:00:00Z",
          captured_at: "2026-01-04T09:00:00Z",
          captured_by: "human:u-1",
          captured_by_name: "Anna Weber",
        },
        allowed_routes: ["privacy_notice"],
        rule: "art14",
        due_at: "2026-02-03T00:00:00Z",
        state: "open",
        attempts: 0,
        created_at: "2026-01-04T09:00:00Z",
      },
    ]),
  "GET /users": () =>
    page([
      {
        id: "u-1",
        email: "anna@margince.test",
        display_name: "Anna Weber",
        status: "active",
        is_agent: false,
      },
    ]),
  "GET /users/names": () =>
    jsonResponse({ data: [{ id: "u-1", display_name: "Anna Weber" }] }),
};

// The privacy officer: every lane's read, and the writes each lane offers.
const OFFICER: GrantSpec = {
  consent_config: ["create"],
  retention_policy: ["read", "create", "update", "delete"],
  privacy_request: ["read", "update"],
  contact: ["update"],
};

function lanes() {
  stubWithSession(ROUTES, OFFICER);
  return (
    <StoryProviders>
      <PrivacyLanes />
    </StoryProviders>
  );
}

const meta: Meta<typeof PrivacyLanes> = {
  title: "Settings/Governance/Privacy and retention/Page",
  component: PrivacyLanes,
};
export default meta;

type Story = StoryObj<typeof PrivacyLanes>;

export const Populated: Story = { render: lanes };

export const PopulatedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: lanes,
};
