/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ContractTerms } from "./contractterms";

type Contract = components["schemas"]["Contract"];

afterEach(cleanup);

// The two readings this component adds were writable from the contract form
// and visible nowhere afterwards. What it must not do is collapse the three
// states a payment term has: a number, a nought, and nothing recorded.

// Every required field, because `Contract` is the wire shape and a fixture
// that omits half of it is not the thing the component is handed. The
// typecheck that reads the test files catches the omission; the one that
// reads only the app does not.
const BASE: Contract = {
  id: "k-1",
  company_id: "o-1",
  title: "Platform subscription",
  status: "active",
  version: 1,
  value_basis: "total",
  auto_renew: false,
  // DERIVED from the dates by the server, not asserted by a human. False here
  // because this fixture records no term at all.
  under_contract: false,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function draw(contract: Contract) {
  return render(
    <LocaleProvider>
      <ContractTerms contract={contract} />
    </LocaleProvider>,
  );
}

it("states a payment term in the words the paper uses", () => {
  draw({ ...BASE, payment_term_days: 30 });
  expect(screen.getByText("Net 30")).toBeTruthy();
});

it("keeps due-on-receipt apart from no terms at all", () => {
  // The form's own hint draws this line: 0 means the invoice is due when it
  // arrives, and blank means nobody agreed terms. Rendering the first as an
  // empty caption would say an agreement demanding immediate payment has no
  // terms — the opposite of what somebody negotiated.
  draw({ ...BASE, payment_term_days: 0 });
  expect(screen.getByText("Due on receipt")).toBeTruthy();
});

it("says nothing at all when no term was recorded", () => {
  const { container } = draw(BASE);
  expect(container.textContent).toBe("");
});

it("reads the recurring value per year and per month", () => {
  // 120000 minor units is €1,200.00, and a twelfth of it is €100.00 exactly —
  // so this case also proves an exact division carries no approximation mark.
  const { container } = draw({ ...BASE, arr_minor: 120000, currency: "EUR" });
  expect(container.textContent).toContain("\u20ac1,200.00/year");
  expect(container.textContent).toContain("(\u20ac100.00/month)");
  expect(container.textContent).not.toContain("approx");
});

it("marks a monthly figure the division could not state exactly", () => {
  // Twelve of an approximate monthly figure do not add back to the year, and
  // a reader who multiplies it should not be surprised by the difference.
  draw({ ...BASE, arr_minor: 100, currency: "EUR" });
  expect(screen.getByText(/approx/)).toBeTruthy();
});

it("withholds the recurring value when nothing prices it", () => {
  // An annual figure with no currency is a number a reader will read in
  // whatever currency the row beside it showed.
  const { container } = draw({ ...BASE, arr_minor: 120000 });
  expect(container.textContent).toBe("");
});
