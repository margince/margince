// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { useLocale, useT } from "../i18n";
import { ProjectContractRow } from "./contractterms";
import { StoryProviders } from "./story-utils";
import "./projects.css";

// One agreement per row on the project's contracts card: its status badge, its
// value, when it ends, and the terms that no other read surface shows. The
// three rows keep apart the three states a payment term has — a number, due on
// receipt, and nothing agreed — because the last two look alike at a glance.

type Contract = components["schemas"]["Contract"];

const BASE: Contract = {
  id: "k-1",
  company_id: "o-1",
  project_id: "p-1",
  title: "Platform subscription",
  status: "active",
  version: 1,
  value_basis: "total",
  value_minor: 4_800_000,
  currency: "EUR",
  arr_minor: 1_600_000,
  starts_on: "2026-01-01",
  ends_on: "2028-12-31",
  auto_renew: false,
  payment_term_days: 30,
  under_contract: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const CONTRACTS: readonly Contract[] = [
  BASE,
  {
    ...BASE,
    id: "k-2",
    title: "Rollout services",
    status: "draft",
    arr_minor: null,
    value_minor: 350_000,
    ends_on: "2026-12-31",
    payment_term_days: 0,
  },
  {
    ...BASE,
    id: "k-3",
    title: "Pilot agreement",
    status: "expired",
    arr_minor: null,
    value_minor: null,
    currency: null,
    ends_on: "2025-12-31",
    payment_term_days: null,
    under_contract: false,
  },
];

function ContractsCard() {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("project.contracts.title")}>
      {CONTRACTS.map((contract) => (
        <ProjectContractRow
          key={contract.id}
          contract={contract}
          locale={locale}
        />
      ))}
    </Panel>
  );
}

const meta: Meta = {
  title: "Records/Project/Contracts",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const ThreeTerms: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <ContractsCard />
      </div>
    </StoryProviders>
  ),
};

export const ThreeTermsDark: Story = {
  globals: { theme: "dark" },
  render: ThreeTerms.render,
};
