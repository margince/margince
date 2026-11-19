/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { Employers } from "./contactemployers";

afterEach(cleanup);

const page = { has_more: false, next_cursor: null };

// A contact who works at Employer GmbH now: a current role there is not
// offered twice, and a past role there is.
it("offers a company already on the list for a former role only", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: "o-9", display_name: "Employer GmbH" }],
        page,
      }),
    "GET /companies/o-9": () =>
      jsonResponse({ id: "o-9", display_name: "Employer GmbH" }),
    "GET /contacts/p-1/relationships": () => jsonResponse({ data: [], page }),
  });
  render(
    <StoryProviders>
      <Employers
        view={{
          ...view,
          employments: {
            data: [
              {
                relationship_id: "rel-9",
                company_id: "o-9",
                company_name: "Employer GmbH",
                is_current_primary: true,
                started_at: "2022-03-01T00:00:00Z",
                ended_at: null,
              },
            ],
            page,
          },
        }}
      />
    </StoryProviders>,
  );
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  await user.type(screen.getByRole("searchbox"), "emp");
  expect(
    await screen.findByText(en["contact.rail.allCompaniesConnected"]),
  ).toBeTruthy();
  await user.click(screen.getByRole("combobox", { name: "Employment status" }));
  await user.click(screen.getByRole("option", { name: "Former" }));
  const dialog = screen.getByRole("dialog");
  expect(await within(dialog).findByText("Employer GmbH")).toBeTruthy();
  expect(
    within(dialog).queryByText(en["contact.rail.allCompaniesConnected"]),
  ).toBeNull();
});
