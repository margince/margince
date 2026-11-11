/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { mount, view } from "../../testing/contactpage.testkit";
import { jsonResponse } from "../../testing/story-utils";

afterEach(cleanup);

// Where a contact works and worked has its own tab. A reader looking for a
// past employer finds a tab named for companies, not one named for people.

const peopleRoutes = {
  "GET /contacts/p-1/graph": () =>
    jsonResponse({
      contact_id: "p-1",
      nodes: [],
      edges: [],
      routes: [],
      groups_omitted: [],
    }),
  "GET /contacts/p-1/network": () =>
    jsonResponse({
      contact_id: "p-1",
      colleagues: [],
      capture_gaps: [],
      capture_gaps_total: 0,
    }),
  "GET /contacts/p-1/intro-requests": () => jsonResponse({ data: [] }),
};

// Two roles at one company and one at another: two companies on the tab.
const employed = {
  ...view,
  employments: {
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
    page: { has_more: false },
  },
};

describe("the contact's Companies tab", () => {
  it("names the tab for companies and counts the employers", async () => {
    mount("companies", employed);

    const tab = await screen.findByRole("button", {
      name: new RegExp(`^${en["tab.companies"]}`),
    });
    expect(tab.getAttribute("aria-pressed")).toBe("true");
    expect(within(tab).getByText("2")).toBeTruthy();
  });

  it("draws the work history with the verb to add a company", async () => {
    mount("companies", employed);

    expect(
      await screen.findByRole("heading", {
        name: en["contact.rail.employmentTitle"],
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["contact.rail.addEmployment"] }),
    ).toBeTruthy();
  });

  it("leaves the work history off the Relationships tab", async () => {
    mount("network", employed, [], peopleRoutes);

    const tab = await screen.findByRole("button", { name: en["tab.network"] });
    expect(tab.getAttribute("aria-pressed")).toBe("true");
    expect(
      screen.queryByRole("heading", {
        name: en["contact.rail.employmentTitle"],
      }),
    ).toBeNull();
  });
});
