import type { components } from "../../api/schema";
import { project, project360 } from "../../testing/projects.fixtures";

// A project well into delivery, for the How it stands stories. It has two phases
// behind it, a won and an open deal, an active contract, a late task and a
// week with nothing filed. Built on the shared 360 fixture so it cannot drift
// from the one the page tests use.

type Project360 = components["schemas"]["Project360"];

const page = { next_cursor: null, has_more: false };

export function deliveringProject360(
  overrides: Partial<Project360> = {},
): Project360 {
  return project360({
    as_of: "2026-08-13T10:00:00Z",
    project: project({
      name: "Depot rollout",
      key: "DR-1",
      phase: "delivering",
      started_at: "2026-03-01",
      target_end_date: "2026-11-30",
      last_activity_at: "2026-08-06T09:00:00Z",
    }),
    phase_history: {
      data: [
        {
          id: "ph-1",
          from_phase: null,
          to_phase: "initiative",
          reason: null,
          changed_at: "2026-03-01T09:00:00Z",
          changed_by: {
            actor_type: "human",
            actor_id: "human:u-me",
            actor_name: "Lena Fischer",
          },
        },
        {
          id: "ph-2",
          from_phase: "initiative",
          to_phase: "delivering",
          reason: "Contract signed",
          changed_at: "2026-04-01T09:00:00Z",
          changed_by: {
            actor_type: "human",
            actor_id: "human:u-me",
            actor_name: "Lena Fischer",
          },
        },
      ],
      phase_durations: [
        { phase: "initiative", seconds: 31 * 86_400, current: false },
        { phase: "delivering", seconds: 134 * 86_400, current: true },
      ],
    },
    deals: {
      data: [
        {
          id: "d-1",
          name: "Phase one licence",
          pipeline_id: "pl",
          stage_id: "s-won",
          status: "won",
          closed_at: "2026-06-02T09:00:00Z",
          amount_minor: 450_000,
          currency: "EUR",
          source: "manual",
          captured_by: "u-me",
          created_at: "2026-03-10T09:00:00Z",
          updated_at: "2026-06-02T09:00:00Z",
        },
        {
          id: "d-2",
          name: "Phase two services",
          pipeline_id: "pl",
          stage_id: "s-discovery",
          status: "open",
          amount_minor: 1_200_000,
          currency: "EUR",
          source: "manual",
          captured_by: "u-me",
          created_at: "2026-06-05T09:00:00Z",
          updated_at: "2026-07-20T09:00:00Z",
        },
      ],
      page,
    },
    contracts: {
      data: [
        {
          id: "c-1",
          company_id: "o-1",
          project_id: "pr-1",
          title: "Master services agreement",
          contract_number: "MSA-2026-01",
          status: "active",
          signed_on: "2026-04-01",
          ends_on: "2027-03-31",
          source: "manual",
          captured_by: "human:u-me",
          under_contract: true,
          auto_renew: false,
          value_basis: "total",
          version: 1,
          created_at: "2026-04-01T09:00:00Z",
          updated_at: "2026-04-01T09:00:00Z",
        },
      ],
      page,
    },
    documents: {
      data: [
        {
          id: "att-1",
          entity_type: "project",
          entity_id: "pr-1",
          filename: "site-report.pdf",
          title: "Site report",
          byte_size: 482_000,
          source: "manual",
          captured_by: "human:u-me",
          created_at: "2026-08-06T09:00:00Z",
        },
      ],
      page,
    },
    commitments: {
      data: [
        {
          activity_id: "a-1",
          subject: "Send the kickoff agenda",
          due_at: "2026-07-03T09:00:00Z",
          assignee_id: "u-me",
          assignee_name: "Lena Fischer",
          overdue: true,
        },
      ],
      page,
    },
    rollups: {
      open_deal_value: { amount_minor: 1_200_000, currency: "EUR" },
      won_deal_value: { amount_minor: 450_000, currency: "EUR" },
      open_commitments: 3,
      last_activity_at: "2026-08-06T09:00:00Z",
      activity_count: 38,
    },
    health: {
      current: {
        id: "h-1",
        project_id: "pr-1",
        state: "at_risk",
        note: "Delivery is running, but the kickoff agenda is late and nothing has been logged for a week.",
        assessed_at: "2026-08-12T09:00:00Z",
        source: "human",
        source_author: "Lena Fischer",
        created_at: "2026-08-12T09:00:00Z",
        superseded: false,
      },
    },
    ...overrides,
  });
}
