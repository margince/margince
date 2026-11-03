// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { ConnectTiles } from "../features/capture/connection-empty";
import { CompanyOverviewCards } from "../features/companies/companyoverviewcards";
import { SignalsSection } from "../features/companies/companysignals";
import { ProjectCompanies } from "../features/projects/projectcompanies";
import { company360 } from "../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { AgentPanel } from "./agentpanel";
import { RailLine } from "./agentrail-line";
import type { AiActivityItem } from "./agentrail-reads";
import { Recap } from "./agentrecap";
import "./agentrail.css";

const ID = "01a00000-0000-7000-8000-000000000001";
const line = {
  before: "I read ",
  subject: { name: "Example Works", route: { screen: "companies", id: ID } },
  after: ".",
} as const;
const settled: readonly AiActivityItem[] = [
  {
    id: ID,
    kind: "site_read",
    state: "done",
    started_at: "2026-10-01T09:00:00Z",
    finished_at: "2026-10-01T09:01:00Z",
    subject_label: "Example Works",
    subject_type: "company",
    subject_id: ID,
  },
];
const meta: Meta = {
  title: "Shell/Record links",
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({
          company: ["read"],
          capture_settings: ["update"],
          ai_diagnostics: ["read"],
        }),
        "GET /signals": () =>
          jsonResponse({
            data: [
              {
                id: ID,
                entity_type: "project",
                entity_id: ID,
                kind: "commitment",
                severity: "low",
                summary: "Confirm the next meeting",
                detected_at: "2026-10-01T09:00:00Z",
                evidence: [],
              },
            ],
            page: { has_more: false, next_cursor: null },
          }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj;
export const SpokenRecord: Story = { render: () => <RailLine line={line} /> };
export const SettledRecords: Story = {
  render: () => <Recap settled={settled} />,
};
function Panel() {
  const panel = useRef<HTMLElement>(null);
  return (
    <AgentPanel
      state="idle"
      line={line}
      running={[]}
      unnamed={false}
      settled={settled}
      signals={{
        waiting: 0,
        offline: [],
        sourcesUnknown: false,
        ai: "configured",
        name: "Margince",
        license: undefined,
        licenseLine: "",
        capture: null,
      }}
      model={{ allowed: false, calls: [] }}
      spend={{ allowed: false, minor: undefined, currency: "EUR" }}
      panel={panel}
      frame={{ left: 12, bottom: 12, width: 420, maxHeight: 700 }}
    />
  );
}
export const AgentReport: Story = { render: () => <Panel /> };
export const CompanyCards: Story = {
  render: () => (
    <CompanyOverviewCards
      view={company360}
      onOpenTab={() => {}}
      onOpenTask={() => {}}
      onOpenEmail={() => {}}
    />
  ),
};
export const ProjectSignal: Story = {
  render: () => <SignalsSection companyId={ID} />,
};
export const ProjectEmployer: Story = {
  render: () => (
    <ProjectCompanies
      projectId={ID}
      companies={[
        { company_id: ID, display_name: "Example Works", role: "customer" },
      ]}
      readOnly
    />
  ),
};
export const ConnectionSettings: Story = {
  render: () => (
    <ConnectTiles availability={[]} onPick={() => {}} onImap={() => {}} />
  ),
};
