/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { Employers } from "./contactemployers";

// The picker debounces its search on a timer, so the clock is the test's.
beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

const page = { has_more: false, next_cursor: null };

function openAddCompany(endedAt: string | null = null, routes: RouteMap = {}) {
  installFetchStub({
    ...routes,
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
                is_current_primary: endedAt === null,
                started_at: "2022-03-01T00:00:00Z",
                ended_at: endedAt,
              },
            ],
            page,
          },
        }}
      />
    </StoryProviders>,
  );
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
}

async function chooseStatus(
  user: ReturnType<typeof openAddCompany>,
  name: string,
) {
  await user.click(screen.getByRole("combobox", { name: "Employment status" }));
  await user.click(screen.getByRole("option", { name }));
}

// A contact who works at Employer GmbH now: a current role there is not
// offered twice, and a past role there is.
it("offers a company already on the list for a former role only", async () => {
  const user = openAddCompany();
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  await user.type(screen.getByRole("searchbox"), "emp");
  expect(
    await screen.findByText(en["contact.rail.allCompaniesConnected"]),
  ).toBeTruthy();
  await chooseStatus(user, "Former");
  const dialog = screen.getByRole("dialog");
  expect(await within(dialog).findByText("Employer GmbH")).toBeTruthy();
  expect(
    within(dialog).queryByText(en["contact.rail.allCompaniesConnected"]),
  ).toBeNull();
});

it("lets go of that company when the role turns current again", async () => {
  const user = openAddCompany();
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  await chooseStatus(user, "Former");
  await user.type(screen.getByRole("searchbox"), "emp");
  const dialog = screen.getByRole("dialog");
  await user.click(await within(dialog).findByText("Employer GmbH"));
  await chooseStatus(user, "Current");
  await user.click(within(dialog).getByRole("button", { name: "Create" }));
  expect(within(dialog).getByText("Employer is required.")).toBeTruthy();
  expect(document.activeElement).toBe(within(dialog).getByRole("searchbox"));
});

// A role there that has ended does not keep the company off a current role:
// a contact can go back to a former employer.
it("offers a former employer for a new current role", async () => {
  const user = openAddCompany("2024-06-30T00:00:00Z");
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  await user.type(screen.getByRole("searchbox"), "emp");
  const dialog = screen.getByRole("dialog");
  expect(await within(dialog).findByText("Employer GmbH")).toBeTruthy();
  expect(
    within(dialog).queryByText(en["contact.rail.allCompaniesConnected"]),
  ).toBeNull();
});

it("adds a past job as former, with the end date it was given", async () => {
  const sent: unknown[] = [];
  const user = openAddCompany(null, {
    "POST /relationships": (body) => {
      sent.push(body);
      return jsonResponse({ id: "rel-new" });
    },
  });
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  await chooseStatus(user, "Former");
  await user.type(screen.getByRole("searchbox"), "emp");
  const dialog = screen.getByRole("dialog");
  await user.click(await within(dialog).findByText("Employer GmbH"));
  await user.type(within(dialog).getByLabelText("End date"), "2019-03");
  // A former job cannot be the primary employer, so the box says no.
  expect(
    within(dialog).getByRole("checkbox", {
      name: "This is their primary employer",
    }),
  ).toHaveProperty("checked", false);
  await user.click(within(dialog).getByRole("button", { name: "Create" }));
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({
    employment_status: "former",
    ended_at: "2019-03-01",
    ended_precision: "month",
    is_current_primary: false,
  });
});

it("drops a past end date when the role turns current, and keeps a coming one", async () => {
  const user = openAddCompany();
  await user.click(await screen.findByRole("button", { name: "Add company" }));
  const dialog = screen.getByRole("dialog");
  await chooseStatus(user, "Former");
  await user.type(within(dialog).getByLabelText("End date"), "2019-03");
  await chooseStatus(user, "Current");
  expect(within(dialog).getByLabelText("End date")).toHaveProperty("value", "");

  await user.type(within(dialog).getByLabelText("End date"), "2999-03");
  await chooseStatus(user, "Former");
  await chooseStatus(user, "Current");
  expect(within(dialog).getByLabelText("End date")).toHaveProperty(
    "value",
    "2999-03",
  );
});
