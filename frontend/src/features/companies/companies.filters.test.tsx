/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  company,
  emptyPage,
  jsonResponse,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { openFilterValues } from "../../testing/listquery.testkit";
import { openViewStep } from "../../testing/listview.testkit";
import { CompaniesScreen } from "./companies";
import { render } from "./companies.testkit";

// The Companies list's lifecycle, industry and size dials, and its two tabs.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

function searchOf(url: string): URLSearchParams {
  return new URL(url, window.location.origin).searchParams;
}

describe("CompaniesScreen — the Lifecycle dial", () => {
  it("offers Unknown, the stage every new account starts at, under Any lifecycle", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    const values = within(await openFilterValues(user, "Lifecycle"));
    expect(values.getByRole("radio", { name: "Any lifecycle" })).toBeTruthy();
    await user.click(values.getByRole("radio", { name: "Unknown" }));

    await waitFor(() =>
      expect(
        urls.some((url) => searchOf(url).getAll("status").join() === "unknown"),
      ).toBe(true),
    );
  });
});

describe("CompaniesScreen — the Customers tab", () => {
  it("lists every account we deliver for and keeps the reader's sort", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await user.click(
      screen.getByRole("button", { name: "Sort by Last activity" }),
    );
    await user.click(screen.getByRole("button", { name: "Customers" }));

    await waitFor(() =>
      expect(
        urls.some((url) => {
          const query = searchOf(url);
          return (
            query.getAll("status_any").join() === "onboarding,live,customer" &&
            !query.has("status") &&
            query.get("sort") === "last_activity_at"
          );
        }),
      ).toBe(true),
    );
    // The picked tab claims no order, so it stays lit over the reader's sort.
    expect(
      screen
        .getByRole("button", { name: "Customers" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("keeps the order on Prospects as well", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    await user.click(screen.getByRole("button", { name: "Sort by Company" }));
    await user.click(screen.getByRole("button", { name: "Prospects" }));

    await waitFor(() =>
      expect(
        urls.some((url) => {
          const query = searchOf(url);
          return (
            query.get("status") === "prospect" &&
            query.get("sort") === "display_name"
          );
        }),
      ).toBe(true),
    );
  });
});

describe("CompaniesScreen — industry and size", () => {
  it("narrows to one industry through the server", async () => {
    const user = userEvent.setup();
    const { urls } = stubFetch(async () => emptyPage());
    render(<CompaniesScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    const values = await openFilterValues(user, "Industry");
    await user.click(within(values).getByRole("radio", { name: "Software" }));

    await waitFor(() =>
      expect(
        urls.some((url) => searchOf(url).get("industry") === "software"),
      ).toBe(true),
    );
  });

  it("offers Industry and Employees as columns, worded for the reader", async () => {
    const user = userEvent.setup();
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...company, industry: "software", size_band: "51-200" }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<CompaniesScreen />);
    await screen.findByText("Brandt Automotive GmbH");

    const step = within(await openViewStep(user, "Columns"));
    await user.click(step.getByRole("checkbox", { name: "Industry" }));
    await user.click(step.getByRole("checkbox", { name: "Employees" }));

    const table = within(screen.getByRole("table"));
    expect(await table.findByText("Software")).toBeTruthy();
    expect(table.getByText("51–200 employees")).toBeTruthy();
  });
});
