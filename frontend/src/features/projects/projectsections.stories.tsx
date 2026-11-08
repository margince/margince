// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  CommitmentsCard,
  ProjectContractsCard,
  ProjectDealsCard,
  StakeholdersCard,
} from "./projectsections";
import "./projects.css";

type Contract = components["schemas"]["Contract"];

// The project page's cards on their own, so the rows can be read without the
// rest of the page around them. They are the deals, contracts and commitments of the
// Overview, and the stakeholders the More menu's drawer holds. The whole page
// is `project360.stories.tsx`.
//
// The 360 comes from `projects.fixtures.ts`, the one the project suites build
// from, so these rows cannot drift from what the page itself renders.

const VIEW = project360({
  deals: {
    data: [
      {
        id: "d-1",
        name: "Phase one licence",
        pipeline_id: "pl",
        stage_id: "s3",
        status: "won",
        amount_minor: 450_000,
        currency: "EUR",
        source: "manual",
        captured_by: "u-me",
        created_at: "2026-06-02T09:00:00Z",
        updated_at: "2026-06-02T09:00:00Z",
      },
      {
        id: "d-2",
        name: "Phase two services",
        pipeline_id: "pl",
        stage_id: "s1",
        status: "open",
        amount_minor: 1_200_000,
        currency: "EUR",
        source: "manual",
        captured_by: "u-me",
        created_at: "2026-07-02T09:00:00Z",
        updated_at: "2026-07-02T09:00:00Z",
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
  stakeholders: {
    data: [
      {
        relationship_id: "rel-1",
        contact_id: "p-1",
        contact_name: "Anna Weber",
        role: "project_lead",
      },
      {
        relationship_id: "rel-2",
        contact_id: "p-2",
        contact_name: "Jonas Brandt",
        role: "sponsor",
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
});

const AGREEMENT: Contract = {
  id: "k-1",
  company_id: "o-1",
  project_id: "pr-1",
  title: "Master services agreement",
  status: "active",
  version: 1,
  value_basis: "total",
  value_minor: 4_800_000,
  currency: "EUR",
  arr_minor: 1_600_000,
  payment_term_days: 30,
  signed_on: "2026-04-01",
  starts_on: "2026-04-01",
  ends_on: "2027-03-31",
  auto_renew: false,
  under_contract: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-04-01T00:00:00Z",
  updated_at: "2026-04-01T00:00:00Z",
};

// The three states a payment term has (a number, due on receipt, nothing
// agreed), with and without a yearly value.
const TERMS = project360({
  contracts: {
    data: [
      AGREEMENT,
      {
        ...AGREEMENT,
        id: "k-2",
        title: "Rollout services",
        status: "draft",
        value_minor: 350_000,
        arr_minor: null,
        payment_term_days: 0,
        signed_on: null,
        ends_on: "2026-12-31",
      },
      {
        ...AGREEMENT,
        id: "k-3",
        title: "Pilot agreement",
        status: "expired",
        value_minor: null,
        currency: null,
        arr_minor: null,
        payment_term_days: null,
        signed_on: null,
        starts_on: null,
        ends_on: null,
        under_contract: false,
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
});

const PAPER = project360({
  contracts: {
    data: [AGREEMENT],
    page: { next_cursor: null, has_more: false },
  },
  commitments: {
    data: [
      {
        activity_id: "t-1",
        subject: "Send the kickoff agenda",
        due_at: "2026-07-03T09:00:00Z",
        assignee_id: "u-me",
        assignee_name: "Lena Fischer",
        overdue: true,
      },
    ],
    page: { next_cursor: null, has_more: false },
  },
});

const meta: Meta = {
  title: "Records/Project 360/Sections",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** A won deal with its day beside an open one with its stage, the amount at
 *  the right of each. */
export const Deals: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ deal: ["read"] }) });
    return (
      <StoryProviders>
        <ProjectDealsCard view={VIEW} />
      </StoryProviders>
    );
  },
};

/** The seats on the project, read-only so the rows carry only the role. */
export const Stakeholders: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ project: ["read"] }) });
    return (
      <StoryProviders>
        <StakeholdersCard view={VIEW} projectId="pr-1" readOnly />
      </StoryProviders>
    );
  },
};

/** An active agreement: when it was signed, how long it runs, its payment
 *  term and its yearly value. */
export const Contracts: Story = {
  render: () => (
    <StoryProviders>
      <ProjectContractsCard view={PAPER} onOpenDocuments={() => {}} />
    </StoryProviders>
  ),
};

/** Net 30 with a yearly value, due on receipt with none, and an agreement
 *  with no terms or dates, whose row adds no line. The company's Contracts
 *  panel reads the same words. */
export const ContractTerms: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <ProjectContractsCard view={TERMS} onOpenDocuments={() => {}} />
      </div>
    </StoryProviders>
  ),
};

export const ContractTermsDark: Story = {
  globals: { theme: "dark" },
  render: ContractTerms.render,
};

/** A late task of the reader's own, with its lateness at the right. */
export const Commitments: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
    return (
      <StoryProviders>
        <CommitmentsCard
          view={PAPER}
          meId="u-me"
          onOpenTask={() => {}}
          onOpenAll={() => {}}
        />
      </StoryProviders>
    );
  },
};
