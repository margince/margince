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

const BASE: Contract = {
  id: "k-1",
  company_id: "o-1",
  project_id: "pr-1",
  title: "Platform subscription",
  status: "draft",
  version: 1,
  value_basis: "total",
  currency: "EUR",
  auto_renew: false,
  under_contract: false,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// 1,000,000 minor units does not divide by twelve, so the monthly reading
// carries its approximation mark on both surfaces.
const WITH_TERMS: Contract = {
  ...BASE,
  arr_minor: 1_000_000,
  payment_term_days: 30,
};
const NO_TERMS: Contract = {
  ...BASE,
  id: "k-2",
  title: "Pilot agreement",
  arr_minor: null,
  payment_term_days: null,
};

const NET = "Net 30";
const ARR = "€10,000.00/year (approx. €833.33/month)";

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

function captions(row: HTMLElement, selector: string): string[] {
  return [...row.querySelectorAll(selector)].map((el) => el.textContent ?? "");
}

it("shows an agreement's payment term and yearly value on the project card", () => {
  drawProject([WITH_TERMS]);
  const row = projectRow(WITH_TERMS.title);
  expect(within(row).getByText(NET)).toBeTruthy();
  expect(within(row).getByText(ARR)).toBeTruthy();
});

it("adds no line to a project card row whose agreement has neither term", () => {
  drawProject([
    NO_TERMS,
    { ...NO_TERMS, id: "k-3", title: "Dated pilot", ends_on: "2026-12-31" },
  ]);
  expect(
    projectRow(NO_TERMS.title).querySelector(".overviewrow-sub"),
  ).toBeNull();
  expect(
    projectRow("Dated pilot").querySelector(".overviewrow-sub")?.textContent,
  ).toBe("Runs to 31 Dec 2026");
});

it("reads the terms in the same words as the company's Contracts panel", async () => {
  installFetchStub({
    "GET /me": meRoute({ contract: ["read"] }),
    "GET /companies/o-1/contracts": () =>
      jsonResponse({
        data: [WITH_TERMS],
        page: { next_cursor: null, has_more: false },
      }),
  });
  const company = render(
    <StoryProviders>
      <CompanyContractsCard companyId="o-1" />
    </StoryProviders>,
  );
  const companyRow = (await company.findByText(WITH_TERMS.title)).closest(
    ".rec-row",
  );
  if (!(companyRow instanceof HTMLElement)) throw new Error("no company row");
  const companyTerms = captions(companyRow, ".rec-meta > .t-caption");
  cleanup();

  drawProject([WITH_TERMS]);
  const projectTerms = captions(
    projectRow(WITH_TERMS.title),
    ".project-contract-sub > .t-caption",
  );
  expect(companyTerms).toEqual([NET, ARR]);
  expect(projectTerms).toEqual(companyTerms);
});
