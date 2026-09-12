/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type BillingContact, BillingContactsPanel } from "./billingcontacts";
import { ContactBillingRoles } from "./contactbillingroles";

// The rule this panel turns on is the difference between WITHHELD and EMPTY,
// and it is the one that fails silently: both render as "no rows", and a card
// that shows "nobody is named" to a reader who simply lacks the grant states a
// fact about the account that nobody established.

afterEach(cleanup);

function render(ui: React.ReactNode) {
  return rtlRender(<LocaleProvider>{ui}</LocaleProvider>);
}

const PAT: BillingContact = {
  relationship_id: "r-1",
  contact_id: "c-1",
  full_name: "Pat Okafor",
  role: "recipient",
  email: "pat@acme.test",
};

it("renders each billing contact with their capacity and address", () => {
  render(<BillingContactsPanel contacts={[PAT]} />);
  expect(screen.getByText("Pat Okafor")).toBeTruthy();
  expect(screen.getByText("Invoice recipient")).toBeTruthy();
  expect(screen.getByRole("link", { name: "pat@acme.test" })).toBeTruthy();
});

it("says so when a named recipient has nowhere to send the invoice", () => {
  render(<BillingContactsPanel contacts={[{ ...PAT, email: null }]} />);
  // Stated rather than left blank: a missing address is the one thing about a
  // billing contact that stops the invoice arriving, and an empty line reads
  // as fine to somebody scanning the list.
  expect(screen.getByText("No email recorded")).toBeTruthy();
});

it("distinguishes nobody named from not allowed to see", () => {
  const { unmount } = render(<BillingContactsPanel contacts={[]} />);
  // Empty is a real answer about the account and says so, because a paying
  // customer with no recipient on file is a gap somebody should close.
  expect(screen.getByText(/Nobody is named yet/)).toBeTruthy();
  unmount();

  // Undefined means the server withheld the section. The panel does not
  // appear at all — claiming "nobody is named" here would be a statement this
  // reader has no standing to make.
  render(<BillingContactsPanel contacts={undefined} />);
  expect(screen.queryByText(/Nobody is named yet/)).toBeNull();
  expect(screen.queryByText("Billing contacts")).toBeNull();
});

it("lists one contact's several capacities in invoice order", () => {
  render(
    <BillingContactsPanel
      contacts={[
        PAT,
        { ...PAT, relationship_id: "r-2", role: "approver" },
        { ...PAT, relationship_id: "r-3", role: "accounts_payable" },
      ]}
    />,
  );
  // A small customer's office manager is often all three, and each capacity is
  // its own row because each is its own edge the panel can change alone.
  expect(screen.getAllByText("Pat Okafor")).toHaveLength(3);
  expect(screen.getByText("Approves")).toBeTruthy();
  expect(screen.getByText("Accounts payable")).toBeTruthy();
});

it("shows nothing on a contact who handles nobody's invoices", () => {
  // Unlike the company panel, an empty list renders nothing: most contacts
  // handle no invoices, and an empty panel on every contact page in the
  // product would be noise stating the obvious.
  render(<ContactBillingRoles companies={[]} />);
  expect(screen.queryByText("Handles invoices for")).toBeNull();
});

it("names the companies a contact bills for", () => {
  render(
    <ContactBillingRoles
      companies={[
        {
          relationship_id: "r-9",
          company_id: "o-1",
          company_name: "Acme",
          role: "approver",
        },
      ]}
    />,
  );
  expect(screen.getByText("Handles invoices for")).toBeTruthy();
  expect(screen.getByText("Acme")).toBeTruthy();
  expect(screen.getByText("Approves")).toBeTruthy();
});
