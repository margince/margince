/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, test } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CompanyPeopleList } from "./contacts";
import { contactsFixture, stubContacts } from "./contacts.fixtures";

// Each test renders its own list; without this the previous test's rows stay in
// the document and a name-based query finds two of everything.
afterEach(() => {
  cleanup();
  window.location.hash = "";
});

function render(ui: ReactNode, locale: "en" | "de" = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The list exists so a rep can tell four states apart at a glance. These tests
// hold the part a payload cannot: that the states reach the screen as different
// words, and that the wire call carries the dials the server actually declares.

test("names each engagement state in its own words", async () => {
  stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />);

  // Four different words, because "their mail sits unanswered", "we replied",
  // "we wrote and heard nothing" and "nobody has tried" are four different
  // next moves. A single label for two of them is the roster this list
  // replaced — and an unanswered inbound wearing "Answered" was the worst of
  // those collapses.
  expect(await screen.findByText("Needs reply")).not.toBeNull();
  expect(screen.getByText("Answered")).not.toBeNull();
  expect(screen.getByText("No reply")).not.toBeNull();
  expect(screen.getByText("Not approached")).not.toBeNull();
});

test("says which side the conversation is owed, not just when it moved", async () => {
  stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />);

  // A date alone reads the same whoever sent it. The direction is the fact.
  expect(await screen.findByText(/They wrote/)).not.toBeNull();
  expect(screen.getAllByText(/We wrote/).length).toBeGreaterThan(0);
  expect(screen.getByText("No exchange yet")).not.toBeNull();
});

test("asks the server for the account's own contacts and lets it choose the order", async () => {
  const calls = stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />);

  await screen.findByText("Dietmar Rietsch");
  const url = calls.at(-1) ?? "";
  expect(url).toContain("/organizations/o-1/contacts");
  // No sort on the first read: the server's own order IS the recommendation,
  // and naming one here would open the page on an alphabet.
  expect(url).not.toContain("sort=");
});

test("renders the German words under a German locale", async () => {
  stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />, "de");

  // The engagement WORDS, not the column head: "Kontaktstand" labels both the
  // column and its filter, and asserting it would pass on the chrome alone.
  await waitFor(() => expect(screen.getByText("Beantwortet")).not.toBeNull());
  expect(screen.getByText("Antwort fällig")).not.toBeNull();
});

test("a second press on a column asks for the reverse, and the server accepts it", async () => {
  const calls = stubContacts(contactsFixture());
  const user = userEvent.setup();
  render(<CompanyPeopleList orgId="o-1" />);

  await screen.findByText("Dietmar Rietsch");
  const header = screen.getByRole("button", { name: "Sort by Last exchange" });

  // A column header is a toggle. The design system spells the reverse by
  // prefixing a minus onto the column's OWN field, so a column declared as
  // `-last_interaction` would ask for `--last_interaction` here — a value the
  // endpoint's enum never declared, answered with a 422 on a control the
  // reader was invited to press.
  await user.click(header);
  await waitFor(() => expect(calls.at(-1)).toContain("sort=-last_interaction"));
  await user.click(header);
  await waitFor(() => expect(calls.at(-1)).toContain("sort=last_interaction"));
  expect(calls.at(-1)).not.toContain("--last_interaction");
});

test("does not let a pasted cursor or limit override the paging it computed", async () => {
  const calls = stubContacts(contactsFixture());
  window.location.hash = "#/companies/o-1/people?cursor=garbage&limit=1";
  render(<CompanyPeopleList orgId="o-1" />);

  await screen.findByText("Dietmar Rietsch");
  // The address is the reader's to edit; the paging keys are not theirs to set.
  expect(calls.at(-1)).not.toContain("cursor=garbage");
  expect(calls.at(-1)).not.toContain("limit=1&");
});
