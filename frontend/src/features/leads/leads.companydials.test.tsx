/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { LeadsScreen } from "./leads";
import {
  emptyPage,
  jsonResponse,
  LEAD_GRANTS,
  lead,
  render,
  stubFetch,
} from "./leads.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
  window.location.hash = "";
});

const COMPANY_DIALS = ["industry", "country", "size_band"];

const onePage = () =>
  jsonResponse({ data: [lead], page: { next_cursor: null, has_more: false } });

// The list as a seat sees it: `refused` is what /me names, and `listAnswer`
// stands in for the server's answer to a narrowed list.
function serve({
  refused,
  listAnswer = onePage,
}: Readonly<{ refused?: string[]; listAnswer?: (url: string) => Response }>) {
  return stubFetch(async (url) => {
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { id: "u-9", display_name: "Me" },
        roles: ["rep"],
        teams: [],
        authorization: meFixture({
          allow: { ...LEAD_GRANTS, company: ["read"] },
          refusedLeadFilters: refused,
        }).authorization,
      });
    }
    if (new URL(url).pathname === "/v1/leads") {
      return listAnswer(url);
    }
    return emptyPage();
  });
}

async function pickFilter(attribute: string, value: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Add filter" }));
  const step = (name: string) => screen.getByRole("group", { name });
  await user.click(
    within(step("Filter")).getByRole("button", { name: attribute }),
  );
  await user.click(within(step(attribute)).getByRole("radio", { name: value }));
}

const leadListQueries = (urls: string[]) =>
  urls
    .map((url) => new URL(url))
    .filter((url) => url.pathname === "/v1/leads")
    .map((url) => url.searchParams);

describe("the lead list's company dials", () => {
  it.each([
    ["Industry", "Logistics and Transport", "industry=logistics_transport"],
    ["Country", "Germany", "country=DE"],
    ["Company size", "51–200 employees", "size_band=51-200"],
  ])("narrows by %s and sends %s", async (attribute, value, sent) => {
    const { urls } = serve({});
    render(<LeadsScreen />);
    await pickFilter(attribute, value);
    await waitFor(() =>
      expect(urls.some((url) => url.includes(sent))).toBe(true),
    );
  });

  it("offers none of the three to a seat refused them", async () => {
    serve({ refused: COMPANY_DIALS });
    render(<LeadsScreen />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Add filter" }),
    );
    const menu = screen.getByRole("group", { name: "Filter" });
    expect(within(menu).getByRole("button", { name: "Status" })).toBeTruthy();
    for (const name of ["Industry", "Country", "Company size"]) {
      expect(within(menu).queryByRole("button", { name })).toBeNull();
    }
  });

  it("names a refused dial an old link carries, and asks for no list until it is removed", async () => {
    window.location.hash = "#/leads?industry=software";
    const { urls } = serve({ refused: COMPANY_DIALS });
    render(<LeadsScreen />);
    expect(
      await screen.findByText("Your role cannot narrow leads by Industry.", {
        exact: false,
      }),
    ).toBeTruthy();
    expect(leadListQueries(urls)).toEqual([]);

    await userEvent.click(
      screen.getByRole("button", { name: "Remove Industry" }),
    );
    expect(await screen.findByText("Jonas Petersen")).toBeTruthy();
    expect(leadListQueries(urls).some((query) => query.has("industry"))).toBe(
      false,
    );
  });

  it("names the dial the server refused before /me knew the role, and reads /me again", async () => {
    window.location.hash = "#/leads?country=DE";
    const { urls } = serve({
      listAnswer: (url) =>
        url.includes("country=DE")
          ? jsonResponse({ status: 403, title: "Forbidden" }, 403)
          : onePage(),
    });
    render(<LeadsScreen />);
    expect(
      await screen.findByText("Your role cannot narrow leads by Country.", {
        exact: false,
      }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(urls.filter((url) => url.endsWith("/v1/me")).length).toBe(2),
    );
  });
});

describe("the New lead form", () => {
  it("files the lead under the industry the form carries", async () => {
    let posted: Record<string, unknown> | null = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/leads")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...lead, id: "l-new" }, 201);
      }
      return emptyPage();
    });
    const user = userEvent.setup();
    render(<LeadsScreen />);
    await user.click(await screen.findByTestId("new-record"));
    const form = within(screen.getByRole("dialog"));
    await user.type(form.getByLabelText("Full name *"), "Otto Fischer");
    await user.click(form.getByRole("combobox", { name: "Industry" }));
    await user.click(await screen.findByRole("option", { name: "Software" }));
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toMatchObject({ industry: "software" }));
  });
});
