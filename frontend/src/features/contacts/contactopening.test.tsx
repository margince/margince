/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import { ContactOpening } from "./contactopening";

const contact = { ...view.contact, title: "Buyer" };
afterEach(cleanup);

function mount(props: Parameters<typeof ContactOpening>[0]) {
  const client = new QueryClient();
  client.setQueryData(["contacts", {}], {
    pages: [{ data: [{ id: contact.id, full_name: "List Name" }] }],
  });
  const opening = (next: Parameters<typeof ContactOpening>[0]) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactOpening {...next} />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const rendered = render(opening(props));
  return (next: Parameters<typeof ContactOpening>[0]) =>
    rendered.rerender(opening(next));
}

it("replaces a cached list name with the confirmed name and title", () => {
  const update = mount({ id: contact.id });
  expect(screen.getByRole("heading", { name: "List Name" })).toBeTruthy();
  update({ id: contact.id, contact });
  expect(screen.getByRole("heading", { name: contact.full_name })).toBeTruthy();
  expect(screen.queryByText("List Name")).toBeNull();
  expect(screen.getByText(contact.title)).toBeTruthy();
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
});

it("does not restore a stale list name when the confirmed name is blank", () => {
  mount({
    id: contact.id,
    contact: { ...contact, full_name: " ", title: null },
  });
  expect(screen.queryByRole("heading")).toBeNull();
  expect(screen.queryByText("List Name")).toBeNull();
  expect(screen.queryByText(contact.title)).toBeNull();
});

it("never shows fields from a contact for another ID", () => {
  mount({ id: "p-2", contact });
  expect(screen.queryByRole("heading")).toBeNull();
  expect(screen.queryByText(contact.title)).toBeNull();
});

it("hides both confirmed fields and cached names after a refused read", () => {
  const update = mount({ id: contact.id, contact, hideIdentity: true });
  expect(screen.queryByRole("heading")).toBeNull();
  expect(screen.queryByText(contact.title)).toBeNull();
  update({ id: contact.id, hideIdentity: true });
  expect(screen.queryByText("List Name")).toBeNull();
  expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
});

it("keeps the title's provider receipt available without record actions", () => {
  mount({
    id: contact.id,
    contact: {
      ...contact,
      writable: false,
      bought_fields: [
        {
          target: "title",
          provider: "surfe",
          paid: true,
          trigger: "manual",
          applied_at: "2026-06-01T08:00:00Z",
        },
      ],
    },
  });
  const receipt = screen.getByRole("button", {
    name: "Where “Buyer” came from",
  });
  expect(screen.getAllByRole("button")).toEqual([receipt]);
  fireEvent.click(receipt);
  expect(screen.getByText(/Surfe/)).toBeTruthy();
  expect(screen.getByText(en["contact.page.loading"])).toBeTruthy();
});
