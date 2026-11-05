/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
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

// The list as a seat sees it: `refused` is what /me names (per read, when it
// changes between reads), and `listAnswer` stands in for the narrowed list.
function serve({
  refused,
  listAnswer = onePage,
}: Readonly<{
  refused?: string[] | ((read: number) => string[]);
  listAnswer?: (url: string) => Response;
}>) {
  let reads = 0;
  return stubFetch(async (url) => {
    if (url.endsWith("/v1/me")) {
      reads += 1;
      return jsonResponse({
        user: { id: "u-9", display_name: "Me" },
        roles: ["rep"],
        teams: [],
        authorization: meFixture({
          allow: { ...LEAD_GRANTS, company: ["read"] },
          refusedLeadFilters:
            typeof refused === "function" ? refused(reads) : refused,
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

  it("names a refused list an old link narrows by, and asks for no wider list", async () => {
    window.location.hash =
      "#/leads?list_id=0b1e2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d";
    const { urls } = serve({ refused: ["list_id"] });
    render(<LeadsScreen />);
    expect(
      await screen.findByText("Your role cannot narrow leads by Lists.", {
        exact: false,
      }),
    ).toBeTruthy();
    expect(leadListQueries(urls)).toEqual([]);
    expect(screen.queryByText("Jonas Petersen")).toBeNull();
  });

  it("asks for the list once /me stops refusing the dial, and shows its rows", async () => {
    window.location.hash = "#/leads?industry=software";
    const { urls } = serve({
      refused: (read) => (read > 1 ? [] : COMPANY_DIALS),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(<LeadsScreen />, client);
    await screen.findByText("Your role cannot narrow leads by Industry.", {
      exact: false,
    });
    expect(leadListQueries(urls)).toEqual([]);

    await client.invalidateQueries({ queryKey: ["me"] });
    expect(await screen.findByText("Jonas Petersen")).toBeTruthy();
    expect(
      leadListQueries(urls).some(
        (query) => query.get("industry") === "software",
      ),
    ).toBe(true);
  });

  it("leaves a refused dial it cannot name to the list's own refusal", async () => {
    window.location.hash = "#/leads?min_score=80";
    serve({
      refused: ["min_score"],
      listAnswer: (url) =>
        url.includes("min_score=80")
          ? jsonResponse({ status: 403, title: "Forbidden" }, 403)
          : onePage(),
    });
    render(<LeadsScreen />);
    expect(
      await screen.findByText("Could not load this view. Reload the page."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Your role cannot narrow leads by", { exact: false }),
    ).toBeNull();
  });

  const forbidden = (url: string) =>
    url.includes("country=DE")
      ? jsonResponse({ status: 403, title: "Forbidden" }, 403)
      : onePage();

  it("reads /me again on a refusal it did not foresee, and names the dial /me then refuses", async () => {
    window.location.hash = "#/leads?country=DE";
    const { urls } = serve({
      refused: (read) => (read > 1 ? COMPANY_DIALS : []),
      listAnswer: forbidden,
    });
    render(<LeadsScreen />);
    expect(
      await screen.findByText("Your role cannot narrow leads by Country.", {
        exact: false,
      }),
    ).toBeTruthy();
    expect(urls.filter((url) => url.endsWith("/v1/me")).length).toBe(2);
  });

  it("does not blame a held dial for a refusal /me still does not explain", async () => {
    window.location.hash = "#/leads?country=DE";
    const { urls } = serve({ listAnswer: forbidden });
    render(<LeadsScreen />);
    await waitFor(() =>
      expect(urls.filter((url) => url.endsWith("/v1/me")).length).toBe(2),
    );
    expect(
      await screen.findByText("Could not load this view. Reload the page."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Your role cannot narrow leads by", { exact: false }),
    ).toBeNull();
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
