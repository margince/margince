/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { CompanyContractsCard } from "../features/companies/companycontracts";
import { ProjectContractsCard } from "../features/projects/projectsections";
import { project360 } from "./projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Contract = components["schemas"]["Contract"];

afterEach(cleanup);

// The company's Contracts panel and the project's Contracts card list the same
// agreements. Each reads an agreement's payment term and yearly value, in the
// same words, from one fixture.

// An active agreement with no dates is under contract: the server derives the
// flag from the status and the dates.
const BASE: Contract = {
  id: "k-base",
  company_id: "o-1",
  project_id: "pr-1",
  title: "Base",
  status: "active",
  version: 1,
  value_basis: "total",
  currency: "EUR",
  arr_minor: null,
  payment_term_days: null,
  auto_renew: false,
  under_contract: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// 1,000,000 minor units does not divide by twelve, so the monthly reading
// carries its approximation mark.
const ARR = "€10,000.00/year (approx. €833.33/month)";

type Case = { contract: Contract; terms: string[] };

function agreement(id: string, fields: Partial<Contract>): Contract {
  return { ...BASE, id, title: `Agreement ${id}`, ...fields };
}

const CASES: Case[] = [
  {
    contract: agreement("term", { payment_term_days: 30 }),
    terms: ["Net 30"],
  },
  {
    contract: agreement("receipt", { payment_term_days: 0 }),
    terms: ["Due on receipt"],
  },
  { contract: agreement("arr", { arr_minor: 1_000_000 }), terms: [ARR] },
  {
    contract: agreement("both", {
      payment_term_days: 30,
      arr_minor: 1_000_000,
    }),
    terms: ["Net 30", ARR],
  },
  { contract: agreement("neither", {}), terms: [] },
  {
    // Its cancellation took effect, so the server reads it as no longer under
    // contract. The terms it was agreed on still read.
    contract: agreement("cancelled", {
      status: "cancelled",
      payment_term_days: 45,
      arr_minor: 1_000_000,
      cancellation_effective_on: "2026-03-31",
      under_contract: false,
    }),
    terms: ["Net 45", ARR],
  },
];

function drawProject(contracts: Contract[]) {
  const view = project360({
    contracts: {
      data: contracts,
      page: { next_cursor: null, has_more: false },
    },
  });
  return render(
    <StoryProviders>
      <ProjectContractsCard view={view} />
    </StoryProviders>,
  );
}

function projectRow(title: string): HTMLElement {
  const row = screen.getByText(title).closest("li");
  if (!row) throw new Error(`no project row for ${title}`);
  return row;
}

function captions(row: Element, selector: string): string[] {
  return [...row.querySelectorAll(selector)].map((el) => el.textContent ?? "");
}

async function companyCaptions(): Promise<Map<string, string[]>> {
  installFetchStub({
    "GET /me": meRoute({ contract: ["read"] }),
    "GET /companies/o-1/contracts": () =>
      jsonResponse({
        data: CASES.map(({ contract }) => contract),
        page: { next_cursor: null, has_more: false },
      }),
  });
  const company = render(
    <StoryProviders>
      <CompanyContractsCard companyId="o-1" />
    </StoryProviders>,
  );
  const read = new Map<string, string[]>();
  for (const { contract } of CASES) {
    const row = (await company.findByText(contract.title)).closest(".rec-row");
    if (!row) throw new Error(`no company row for ${contract.title}`);
    read.set(contract.title, captions(row, ".rec-meta > .t-caption"));
  }
  cleanup();
  return read;
}

it("reads every kind of terms in the same words as the company's Contracts panel", async () => {
  const company = await companyCaptions();
  drawProject(CASES.map(({ contract }) => contract));
  for (const { contract, terms } of CASES) {
    const project = captions(
      projectRow(contract.title),
      ".project-contract-sub > .t-caption",
    );
    expect({ title: contract.title, project }).toEqual({
      title: contract.title,
      project: terms,
    });
    expect({
      title: contract.title,
      company: company.get(contract.title),
    }).toEqual({
      title: contract.title,
      company: terms,
    });
  }
});

it("adds no line to a project card row whose agreement has neither term", () => {
  const neither = agreement("neither", {});
  drawProject([
    neither,
    agreement("dated", { title: "Dated pilot", ends_on: "2026-12-31" }),
  ]);
  expect(
    projectRow(neither.title).querySelector(".overviewrow-sub"),
  ).toBeNull();
  expect(
    projectRow("Dated pilot").querySelector(".overviewrow-sub")?.textContent,
  ).toBe("Runs to 31 Dec 2026");
});

it("shows the terms beside the dates on the project card", () => {
  drawProject([
    agreement("dated", {
      title: "Dated subscription",
      ends_on: "2026-12-31",
      payment_term_days: 30,
    }),
  ]);
  const row = projectRow("Dated subscription");
  expect(within(row).getByText("Runs to 31 Dec 2026")).toBeTruthy();
  expect(within(row).getByText("Net 30")).toBeTruthy();
});
