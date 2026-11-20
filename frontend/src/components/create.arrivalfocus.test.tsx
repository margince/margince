// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { RecordView } from "../design-system/recordview";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { forgetListAddresses } from "../lib/listreturn";
import { announceAddressChanged, useRoute } from "../lib/router";
import { seedWriter } from "../testing/mefixture";
import { CreateAction, type CreateField } from "./create";

afterEach(() => {
  cleanup();
  forgetListAddresses();
  window.location.hash = "";
  announceAddressChanged();
});

const fields: CreateField[] = [
  { key: "name", labelText: "Name", type: "text" },
];

// The list with its create, or the record page once the address names one: the
// two pages a create moves between, through the real router.
function ContactsScreen() {
  const route = useRoute();
  if (route.id === "c1") {
    return <RecordView name="Ada Lovelace" identity="c1" zone={UTC_ZONE} />;
  }
  return (
    <CreateAction
      requires={{ object: "contact", action: "create" }}
      label="New contact"
      fields={fields}
      create={async () => ({ id: "c1" })}
      invalidate="contacts"
      screen="contacts"
    />
  );
}

function mount() {
  return render(
    <QueryClientProvider client={seedWriter(new QueryClient())}>
      <LocaleProvider initial="en">
        <ContactsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("focus after a create opens the new record", () => {
  it("lands on the record's name, not on the top of the page", async () => {
    window.location.hash = "#/contacts";
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole("button", { name: "New contact" }));
    await user.type(
      await screen.findByRole("textbox", { name: /Name/ }),
      "Ada Lovelace{Enter}",
    );

    const name = await screen.findByRole("heading", {
      level: 1,
      name: "Ada Lovelace",
    });
    await waitFor(() => expect(name).toHaveFocus());
  });

  it("leaves focus alone when the record is opened without a create", () => {
    window.location.hash = "#/contacts/c1";
    mount();
    expect(
      screen.getByRole("heading", { level: 1, name: "Ada Lovelace" }),
    ).not.toHaveFocus();
  });
});
