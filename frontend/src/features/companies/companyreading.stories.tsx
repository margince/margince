// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { type CompanyOverviewProps, CompanyReading } from "./companyreading";

// The head of an account's Overview, read off its 360. An archived account
// has no 360, so its page draws none of this; the page story shows that.
const meta: Meta = {
  title: "Records/Company 360/Reading",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const props: CompanyOverviewProps = {
  company,
  view: company360,
  loading: false,
  failed: false,
  onOpenRecord: () => undefined,
  onOpenEmail: () => undefined,
  onOpenTasks: () => undefined,
  onOpenTask: () => undefined,
  onLogActivity: () => undefined,
  onDraftTo: () => undefined,
  onOpenTab: () => undefined,
};

const page = { has_more: false, next_cursor: null };
const written = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

// A customer a month into a renewal: Relationship rated, a promise overdue, a
// walkthrough booked and a close dated. Every part of How it stands draws.
const standing: components["schemas"]["Company360"] = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  last_inbound_at: "2026-08-01T08:00:00Z",
  last_outbound_at: "2026-07-20T10:00:00Z",
  last_contact: {
    at: "2026-08-01T08:00:00Z",
    kind: "email",
    activity_id: "a-wrote",
  },
  health: {
    relationship: {
      rating: "good",
      reason: "Last met them 24 days ago",
      reason_code: "last_met",
      reason_params: { days: 24 },
    },
  },
  state_strip: {
    account: { lifecycle: "customer", relationship_types: ["customer"] },
    engagement: { state: "waiting_on_us" },
    commercial: {
      open_count: 1,
      stalled_count: 0,
      open_pipeline_minor_base: 4_800_000,
      priced_count: 1,
      converted_count: 0,
      base_currency: "EUR",
      next_close_on: "2026-09-30",
    },
  },
  moment: {
    claim_key: "promise",
    evidence_fingerprint: "fp-promise",
    rule: "overdue_promise",
    headline: "Send Dana the renewal paperwork",
    why_now: "Due 25 Jul, promised after the 18 Jun proposal.",
    confidence: "observed_fact",
    evidence: [{ type: "task", id: "t-renewal", label: "Renewal paperwork" }],
    recommended_action: {
      kind: "complete_task",
      label: "Send paperwork",
      state: "available",
    },
  },
  next_steps: {
    data: [
      {
        activity_id: "t-renewal",
        subject: "Renewal paperwork",
        due_at: "2026-07-25T17:00:00Z",
        overdue: true,
      },
      {
        activity_id: "t-brief",
        subject: "Prepare the walkthrough brief",
        due_at: "2026-08-19T17:00:00Z",
        overdue: false,
      },
    ],
    page,
  },
  next_meeting: {
    activity_id: "m-walk",
    starts_at: "2026-08-20T10:00:00Z",
    subject: "Walkthrough",
    participants: [],
  },
  activities: {
    data: [
      {
        offers_email: false,
        id: "a-kickoff",
        kind: "meeting",
        subject: "Kickoff call",
        occurred_at: "2026-06-03T10:00:00Z",
        is_done: true,
        ...written,
      },
      {
        offers_email: false,
        id: "a-visit",
        kind: "note",
        subject: "Site visit",
        occurred_at: "2026-07-20T10:00:00Z",
        is_done: true,
        ...written,
      },
      {
        offers_email: false,
        id: "a-wrote",
        kind: "email",
        direction: "inbound",
        subject: "Re: retrofit timeline",
        occurred_at: "2026-08-01T08:00:00Z",
        is_done: true,
        ...written,
      },
    ],
    page,
  },
};

function Reading(overrides: Partial<CompanyOverviewProps>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    [`GET /companies/${company.id}/finance-summary`]: () =>
      jsonResponse({
        company_id: company.id,
        state: "connected",
        net_invoiced: { amount_minor: 6_420_000, currency: "EUR" },
        overdue: { amount_minor: 0, currency: "EUR" },
        median_days_after_due: 1,
        payment_behaviour: [0, 2, 1, 0],
      }),
  });
  return (
    <StoryProviders>
      <div className="co-overview-stack">
        <CompanyReading {...props} {...overrides} />
      </div>
    </StoryProviders>
  );
}

export const Assembled: Story = { render: () => <Reading /> };

export const Standing: Story = { render: () => <Reading view={standing} /> };

export const Assembling: Story = {
  render: () => <Reading view={undefined} loading />,
};
