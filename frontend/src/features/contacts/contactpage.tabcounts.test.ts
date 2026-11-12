import { describe, expect, it } from "vitest";
import { populated, type View } from "./contactpage.fixtures";
import { contactTabCounts } from "./contactpage.tabcounts";

const roles = (hasMore: boolean): View["deal_roles"] => ({
  data: [
    {
      relationship_id: "r-1",
      deal_id: "d-1",
      deal_title: "Fleet retrofit",
      deal_stage: "Proposal",
      role: "champion",
    },
    {
      relationship_id: "r-2",
      deal_id: "d-2",
      deal_title: "Phase two",
      deal_stage: "Qualification",
      role: "user",
    },
  ],
  page: { has_more: hasMore },
});

// Two roles at one company and one elsewhere: the tab counts two companies.
const jobs = (hasMore: boolean): View["employments"] => ({
  data: [
    {
      relationship_id: "e-1",
      company_id: "o-1",
      company_name: "Brandt Automotive GmbH",
      role: "Head of Fleet",
      is_current_primary: true,
      started_at: "2022-03-01T00:00:00Z",
      ended_at: null,
    },
    {
      relationship_id: "e-2",
      company_id: "o-1",
      company_name: "Brandt Automotive GmbH",
      role: "Fleet Planner",
      is_current_primary: false,
      started_at: "2020-01-01T00:00:00Z",
      ended_at: "2022-02-01T00:00:00Z",
    },
    {
      relationship_id: "e-3",
      company_id: "o-2",
      company_name: "Voss Logistics",
      role: "Fleet Coordinator",
      is_current_primary: false,
      started_at: "2018-01-01T00:00:00Z",
      ended_at: "2019-12-01T00:00:00Z",
    },
  ],
  page: { has_more: hasMore },
});

describe("contactTabCounts", () => {
  it.each([
    {
      name: "a whole deal list and a whole file walk",
      view: {
        ...populated,
        deal_roles: roles(false),
        employments: jobs(false),
      },
      files: 3,
      want: { deals: 2, companies: 2, documents: 3 },
    },
    {
      name: "a deal list the server cut",
      view: { ...populated, deal_roles: roles(true), employments: jobs(true) },
      files: undefined,
      want: {},
    },
    {
      name: "a deal list withheld from the reader, and no files",
      view: {
        ...populated,
        deal_roles: undefined,
        employments: undefined,
        sections_omitted: ["deal_roles" as const, "employments" as const],
      },
      files: 0,
      want: {},
    },
  ])("counts only lists read whole: $name", ({ view, files, want }) => {
    expect(contactTabCounts(view, files)).toEqual(want);
  });
});
