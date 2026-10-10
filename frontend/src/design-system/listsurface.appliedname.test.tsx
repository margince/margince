/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListChip, ListSurface } from "./listsurface";

// A searched chip has no options to look its value up in, so a value that
// arrived in an address is named by what the caller read back, never by its id.

afterEach(cleanup);

const COMPANY_ID = "01a12601-15f9-757e-857e-976fb08a2a96";

function chip(appliedName: string | undefined): ListChip {
  return {
    key: "company_id",
    label: "Company",
    allLabel: "All companies",
    options: [],
    search: () => Promise.resolve([]),
    appliedName,
  };
}

function render(appliedName: string | undefined) {
  return rtlRender(
    <LocaleProvider initial="en">
      <ListSurface
        title="Deals"
        chips={[chip(appliedName)]}
        chosen={{ company_id: COMPANY_ID }}
      />
    </LocaleProvider>,
  );
}

it("names an applied searched value by the name the caller read", () => {
  render("Northwind Traders");

  expect(
    screen.getByRole("group", { name: "Company: Northwind Traders" }),
  ).toBeInTheDocument();
  expect(screen.queryByText(COMPANY_ID)).toBeNull();
});

it("falls back to the id only when the caller has no name to give", () => {
  render(undefined);

  expect(
    screen.getByRole("group", { name: `Company: ${COMPANY_ID}` }),
  ).toBeInTheDocument();
});
