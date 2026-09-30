/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { mount, view } from "./contactpage.testkit";

afterEach(cleanup);
it("does not label a former or unknown company as the contact's current employer", async () => {
  mount("overview", {
    ...view,
    employments: {
      data: [
        {
          relationship_id: "former",
          company_id: "old",
          company_name: "Former Company",
          employment_status: "former",
          is_current_primary: false,
        },
        {
          relationship_id: "unknown",
          company_id: "unknown",
          company_name: "Unknown Company",
          employment_status: "unknown",
          is_current_primary: false,
        },
      ],
      page: { has_more: false },
    },
  });
  const title = await screen.findByRole("heading", { level: 1 });
  const header = title.closest("header");
  expect(header).not.toBeNull();
  expect(header?.textContent).not.toContain("Former Company");
  expect(header?.textContent).not.toContain("Unknown Company");
});

it("keeps unknown roles and additional current jobs on the Employers panel", async () => {
  const { render, within } = await import("@testing-library/react");
  const { Employers } = await import("./contactemployers");
  const { StoryProviders } = await import("./story-utils");
  render(
    <StoryProviders>
      <Employers
        view={{
          ...view,
          employments: {
            page: { has_more: false },
            data: [
              {
                relationship_id: "primary",
                company_id: "one",
                company_name: "Main Employer",
                employment_status: "current",
                is_current_primary: true,
              },
              {
                relationship_id: "unknown",
                company_id: "two",
                company_name: "Uncertain Employer",
                role: "Advisor",
                employment_status: "unknown",
                is_current_primary: false,
              },
              {
                relationship_id: "other",
                company_id: "three",
                company_name: "Second Employer",
                role: "Director",
                employment_status: "current",
                is_current_primary: false,
              },
            ],
          },
        }}
      />
    </StoryProviders>,
  );
  const uncertain = (await screen.findByText("Uncertain Employer")).closest(
    ".pe-employment",
  );
  expect(await screen.findByText("Second Employer")).toBeTruthy();
  expect(uncertain).not.toBeNull();
  expect(
    within(uncertain as HTMLElement).getByText(/Status unknown/),
  ).toBeTruthy();
});
