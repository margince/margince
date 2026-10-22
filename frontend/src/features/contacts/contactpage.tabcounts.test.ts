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

describe("contactTabCounts", () => {
  it.each([
    {
      name: "a whole deal list and a whole file walk",
      view: { ...populated, deal_roles: roles(false) },
      files: 3,
      want: { deals: 2, documents: 3 },
    },
    {
      name: "a deal list the server cut",
      view: { ...populated, deal_roles: roles(true) },
      files: undefined,
      want: {},
    },
    {
      name: "a deal list withheld from the reader, and no files",
      view: {
        ...populated,
        deal_roles: undefined,
        sections_omitted: ["deal_roles" as const],
      },
      files: 0,
      want: {},
    },
  ])("counts only lists read whole: $name", ({ view, files, want }) => {
    expect(contactTabCounts(view, files)).toEqual(want);
  });
});
