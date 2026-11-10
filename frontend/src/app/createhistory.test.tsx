// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { CreateAction, type CreateField } from "../components/create";
import { LocaleProvider } from "../i18n";
import { forgetListAddresses } from "../lib/listreturn";
import { routeHash, useRoute } from "../lib/router";
import { useShortcutDispatcher } from "../lib/shortcuts";
import { CREATE_ID } from "./nav";
import { usePaletteShortcuts } from "./palettehotkey";

// The list as the app mounts it: keyed on its address, so `#/contacts/new`
// remounts it with the form open, and with the palette's keys live.
function ContactsList() {
  const route = useRoute();
  useShortcutDispatcher();
  usePaletteShortcuts(
    false,
    () => {},
    () => {},
    route.screen,
  );
  const fields: CreateField[] = [{ key: "name", labelText: "Name" }];
  return (
    <CreateAction
      key={routeHash(route)}
      label="New contact"
      fields={fields}
      create={async () => ({ id: "c1" })}
      invalidate="contacts"
      screen="contacts"
      listCreate={{ open: route.id === CREATE_ID }}
    />
  );
}

function mount() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ContactsList />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function arrive(hash: string) {
  const moved = new Promise((resolve) =>
    globalThis.addEventListener("hashchange", resolve, { once: true }),
  );
  window.location.hash = hash;
  await moved;
}

async function back(to: string) {
  await act(async () => globalThis.history.back());
  await waitFor(() => expect(window.location.hash).toBe(to));
}

afterEach(() => {
  cleanup();
  forgetListAddresses();
  window.location.hash = "";
});

describe("the create form a list opens on C", () => {
  it("adds no history entry, so Back after a cancel leaves the list", async () => {
    await arrive("#/home");
    await arrive("#/contacts");
    const entries = globalThis.history.length;
    mount();
    const user = userEvent.setup();

    await user.keyboard("c");
    expect(window.location.hash).toBe("#/contacts/new");
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(window.location.hash).toBe("#/contacts");
    expect(globalThis.history.length).toBe(entries);
    await back("#/home");
  });

  it("goes from a new record back to the list", async () => {
    await arrive("#/home");
    await arrive("#/contacts");
    mount();
    const user = userEvent.setup();

    await user.keyboard("c");
    fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(window.location.hash).toBe("#/contacts/c1"));
    await back("#/contacts");
    await back("#/home");
  });

  it("lands a cancelled deep link on the list", async () => {
    await arrive("#/home");
    await arrive("#/contacts/new");
    mount();

    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(window.location.hash).toBe("#/contacts");
    await back("#/home");
  });

  it("opens the form again on a second C", async () => {
    await arrive("#/contacts");
    mount();
    const user = userEvent.setup();

    await user.keyboard("c");
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.keyboard("c");
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });
});
