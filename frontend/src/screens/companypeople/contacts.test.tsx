/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, test } from "vitest";
import { LocaleProvider } from "../../i18n";
import { CompanyPeopleList } from "./contacts";
import { contactsFixture, stubContacts } from "./contacts.fixtures";

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

// The list exists so a rep can tell three states apart at a glance. These tests
// hold the part a payload cannot: that the states reach the screen as different
// words, and that the wire call carries the dials the server actually declares.

test("names each engagement state in its own words", async () => {
  stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />);

  // Three different words, because "they replied", "we wrote and heard
  // nothing" and "nobody has tried" are three different next moves. A single
  // label for two of them is the roster this list replaced.
  expect(await screen.findByText("Answered")).not.toBeNull();
  expect(screen.getByText("No reply")).not.toBeNull();
  expect(screen.getByText("Not approached")).not.toBeNull();
});

test("says which side the conversation is owed, not just when it moved", async () => {
  stubContacts(contactsFixture());
  render(<CompanyPeopleList orgId="o-1" />);

  // A date alone reads the same whoever sent it. The direction is the fact.
  expect(await screen.findByText(/They wrote/)).not.toBeNull();
  expect(screen.getByText(/We wrote/)).not.toBeNull();
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
  await waitFor(() => expect(screen.getByText("Antwortet")).not.toBeNull());
});
