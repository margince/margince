/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { listFetchLimit } from "../../components/listquery";
import { pickOption } from "../../design-system/select-testing";
import {
  company,
  emptyPage,
  jsonResponse,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { CompaniesScreen } from "./companies";
import { render } from "./companies.testkit";

// The companies list, wired as the contacts list is (contacts.test.tsx):
// search, sort and pagination, the rich create modal, and the dedupe link.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("CompaniesScreen — search/sort/pagination (P-14)", () => {
  it("carries the debounced search term into the next fetch", async () => {
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByPlaceholderText("Search"), {
        target: { value: "brandt" },
      });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() =>
      expect(urls.some((url) => url.includes("q=brandt"))).toBe(true),
    );
  });

  it("fetches the next cursor page when the pager steps past the loaded page", async () => {
    const { urls } = stubFetch(async (url) => {
      if (url.includes("cursor=c1")) {
        return jsonResponse({
          data: [{ ...company, id: "o-2", display_name: "Nordwind Logistik" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        data: [company],
        page: { next_cursor: "c1", has_more: true },
      });
    });
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );

    const next = screen.getByRole("button", { name: "Next" });
    expect((next as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(next);

    await waitFor(() =>
      expect(screen.getByText("Nordwind Logistik")).toBeTruthy(),
    );
    expect(urls.some((url) => url.includes("cursor=c1"))).toBe(true);
  });
});

describe("CompaniesScreen — what an account is to us", () => {
  it("names every relationship type the account carries", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [
          {
            ...company,
            lifecycle: "prospect",
            relationship_types: ["partner", "supplier"],
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );

    // Both, not the first: the field is multi-valued because an account really
    // can be two things at once, and showing one would make the other look
    // untrue. The filter offers these values, so the column has to read them
    // back — a narrowed list that cannot say why a row matched is a list the
    // reader has to take on faith.
    expect(screen.getByText("Partner")).toBeTruthy();
    expect(screen.getByText("Supplier")).toBeTruthy();
    // And the two columns stay distinct: an account can be a Partner sitting
    // at Prospect.
    expect(screen.getByText("Prospect")).toBeTruthy();
  });

  it("names an extension's type by its declared label and offers it as a filter", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(
      async () =>
        jsonResponse({
          data: [
            {
              ...company,
              relationship_types: ["supplier"],
              extension_relationship_types: ["ext:acme:reseller"],
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      {
        companyTypes: [
          { value: "ext:acme:reseller", label: "Reseller", unit: "acme" },
        ],
      },
    );
    render(<CompaniesScreen />);
    await waitFor(() => expect(screen.getByText("Reseller")).toBeTruthy());
    expect(screen.getByText("Supplier")).toBeTruthy();
    expect(screen.queryByText("ext:acme:reseller")).toBeNull();

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Relationship type" }));
    await user.click(screen.getByRole("radio", { name: "Reseller" }));
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            new URL(url, window.location.origin).searchParams.get(
              "extension_relationship_type",
            ) === "ext:acme:reseller",
        ),
      ).toBe(true),
    );
  });

  it("leaves the cell empty when the account carries none", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, relationship_types: [] }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    expect(screen.queryByText("Partner")).toBeNull();
  });
});

// Every reader sees every company, so each row says whose it is. A reader on
// the default view must not take a colleague's account for their own.
describe("CompaniesScreen — whose account each row is", () => {
  it("names the owner on each row", async () => {
    stubFetch(async (url) =>
      url.includes("/users")
        ? jsonResponse({
            data: [
              { id: "u-9", email: "lena@x.test", display_name: "Lena F." },
            ],
            page: { next_cursor: null },
          })
        : jsonResponse({
            data: [{ ...company, owner_id: "u-9" }],
            page: { next_cursor: null, has_more: false },
          }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    await waitFor(() => expect(screen.getByText("Lena F.")).toBeTruthy());
  });

  it("says an account is unassigned rather than leaving the owner blank", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, owner_id: null }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    expect(screen.getByText("Unassigned")).toBeTruthy();
  });
});

describe("CompaniesScreen — how many work here, how many deals are open", () => {
  it("shows both counts, zero included", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [
          { ...company, contact_count: 4, open_deal_count: 2 },
          {
            ...company,
            id: "o-2",
            display_name: "Quiet Ltd",
            contact_count: 0,
            open_deal_count: 0,
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    const first = (await screen.findByText("Brandt Automotive GmbH")).closest(
      "tr",
    );
    const quiet = screen.getByText("Quiet Ltd").closest("tr");
    if (!first || !quiet) {
      throw new Error("rows must render inside table rows");
    }
    expect(within(first).getByText("4")).toBeTruthy();
    expect(within(first).getByText("2")).toBeTruthy();
    // Zero is a number: "no contacts" and "no open deals" are facts, and a
    // blank cell would read as "not shown".
    expect(within(quiet).getAllByText("0").length).toBe(2);
  });

  it("leaves the deal count blank when the server withheld it", async () => {
    // A role without computed_field:read gets no key at all (STATE-4), and
    // the column must not turn that absence into a confident 0.
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, contact_count: 3 }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    const row = (await screen.findByText("Brandt Automotive GmbH")).closest(
      "tr",
    );
    if (!row) {
      throw new Error("row must render inside a table row");
    }
    expect(within(row).getByText("3")).toBeTruthy();
    expect(within(row).queryByText("0")).toBeNull();
  });
});

describe("CompaniesScreen — list dials reach the server (P-14)", () => {
  it("asks the server to re-sort instead of reordering the rows it holds", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await user.click(screen.getByRole("button", { name: "Sort by Company" }));

    // The rows the browser holds are one page of a keyset walk, so a sort that
    // only reordered them would rank a slice and present it as the whole list.
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("sort=display_name") && !url.includes("cursor="),
        ),
      ).toBe(true),
    );
  });

  it("sorts by last activity on the server, not over the rows it holds", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, last_activity_at: "2026-08-10T12:00:00Z" }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );

    await user.click(
      screen.getByRole("button", { name: "Sort by Last activity" }),
    );
    // The server owns the order: a fresh keyset walk under the new sort.
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("sort=last_activity_at") && !url.includes("cursor="),
        ),
      ).toBe(true),
    );
  });

  it("narrows to one lifecycle stage through the server", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await user.click(screen.getByRole("button", { name: "Customers" }));

    await waitFor(() =>
      expect(urls.some((url) => url.includes("lifecycle=customer"))).toBe(true),
    );
  });

  it("narrows to one company size through the server", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Employees" }));
    await user.click(screen.getByRole("radio", { name: "51–200 employees" }));

    // The wire's own parameter name and the band as written. A dial that sent
    // anything else would be ignored by the server, which answers the WHOLE
    // list with 200 OK — a filter that reads as working and is not.
    // The parsed parameter, not a substring of the URL: `includes` would also
    // accept a longer value that merely starts this way, so it asserts less than
    // it appears to — and it would keep passing if the band set ever grew one.
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            new URL(url, window.location.origin).searchParams.get(
              "size_band",
            ) === "51-200",
        ),
      ).toBe(true),
    );
  });

  it("reads whole rendered pages at whatever size the reader picked", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    // One read carries several rendered pages, so the limit is a multiple of
    // the footer's size and never the size itself — that is what lets the
    // pager offer page 2 before the reader has asked for it.
    await waitFor(() =>
      expect(
        urls.some((url) => url.includes(`limit=${listFetchLimit(25)}`)),
      ).toBe(true),
    );

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Rows per page" }),
      "50 per page",
    );

    // The read follows the reader's size rather than a literal. It did not:
    // the screen asked for 50 and drew 25, and said so — "1-25 of 50 loaded
    // so far".
    const resized = `limit=${listFetchLimit(50)}`;
    await waitFor(() =>
      expect(urls.some((url) => url.includes(resized))).toBe(true),
    );

    // A new page size restarts the keyset walk. Carrying the old cursor over
    // would continue a walk taken at the previous size, so the reader would
    // resume mid-list while believing they were on page one.
    expect(
      urls
        .filter((url) => url.includes(resized))
        .every((url) => !url.includes("cursor=")),
    ).toBe(true);
  });
});

describe("CompaniesScreen — rich create (P-15)", () => {
  it("posts display_name + size_band + domains + source:manual on submit", async () => {
    const user = userEvent.setup();
    let posted: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/companies")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...company, id: "o-new" }, 201);
      }
      return emptyPage();
    });
    render(<CompaniesScreen />);
    await user.click(screen.getByTestId("new-record"));
    await user.type(
      screen.getByLabelText("Company name *"),
      "Otto Fischer GmbH",
    );
    await pickOption(
      user,
      screen.getByLabelText("Employees"),
      "11–50 employees",
    );
    await user.click(screen.getByText("Add domain"));
    await user.type(screen.getByLabelText("Domain *"), "otto.example");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      display_name: "Otto Fischer GmbH",
      size_band: "11-50",
      source: "manual",
      domains: [{ domain: "otto.example", is_primary: false }],
    });
  });
});

describe("CompaniesScreen — archived marking (P-3)", () => {
  it("shows an Archived badge on a row with archived_at set", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, archived_at: "2026-07-01T00:00:00Z" }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    expect(screen.getByText("Archived")).toBeTruthy();
  });
});

describe("CompaniesScreen — dedupe view-existing link (P-16)", () => {
  it("renders a link to the collided record on a duplicate_domain 409", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.includes("/companies")) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            detail: "domain already in use",
            code: "duplicate_domain",
            details: { existing_id: "01X" },
          },
          409,
        );
      }
      return emptyPage();
    });
    render(<CompaniesScreen />);
    await userEvent.click(screen.getByTestId("new-record"));
    await userEvent.type(
      screen.getByLabelText("Company name *"),
      "Dup Company",
    );
    await userEvent.click(screen.getByText("Add domain"));
    await userEvent.type(screen.getByLabelText("Domain *"), "dup.example");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(screen.getByText("View existing record")).toBeTruthy(),
    );
    await userEvent.click(screen.getByText("View existing record"));
    expect(window.location.hash).toBe("#/companies/01X");
  });
});
