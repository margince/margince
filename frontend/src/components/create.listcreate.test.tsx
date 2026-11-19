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
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { forgetListAddresses, rememberListAddress } from "../lib/listreturn";
import { ScreenAddress } from "../lib/screenaddress";
import { setSingleKeyShortcutsOn } from "../lib/shortcuts";
import { meFixture } from "../testing/mefixture";
import { CreateAction, type CreateField } from "./create";

afterEach(() => {
  cleanup();
  forgetListAddresses();
  act(() => setSingleKeyShortcutsOn(true));
  vi.restoreAllMocks();
  window.location.hash = "";
});

const fields: CreateField[] = [
  { key: "name", labelText: "Name", type: "text" },
];

function mount(
  props: Readonly<{
    listCreate?: { open: boolean };
    create?: () => Promise<{ id: string }>;
    drawnFor?: string;
    client?: QueryClient;
  }>,
) {
  return render(
    <QueryClientProvider client={props.client ?? new QueryClient()}>
      <LocaleProvider initial="en">
        <ScreenAddress.Provider value={props.drawnFor ?? null}>
          <CreateAction
            requires={{ object: "contact", action: "create" }}
            label="New contact"
            fields={fields}
            create={props.create ?? (async () => ({ id: "c1" }))}
            invalidate="contacts"
            screen="contacts"
            listCreate={props.listCreate}
          />
        </ScreenAddress.Provider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a list's create at its own address", () => {
  it("opens with the form up", async () => {
    window.location.hash = "#/contacts/new";
    mount({ listCreate: { open: true } });
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });

  it("puts the address back on the list when cancelled, in place", async () => {
    window.location.hash = "#/contacts/new";
    const replace = vi.spyOn(globalThis.history, "replaceState");
    mount({ listCreate: { open: true } });
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(window.location.hash).toBe("#/contacts");
    expect(replace.mock.calls.map((call) => call[2])).toContain("#/contacts");
  });

  it("goes back to the list as the reader left it", async () => {
    rememberListAddress("#/contacts?owner=me");
    window.location.hash = "#/contacts/new";
    mount({ listCreate: { open: true } });
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(window.location.hash).toBe("#/contacts?owner=me");
  });

  it("leaves no address behind that opens the form once the record exists", async () => {
    window.location.hash = "#/contacts/new";
    const replace = vi.spyOn(globalThis.history, "replaceState");
    mount({ listCreate: { open: true } });
    fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(window.location.hash).toBe("#/contacts/c1"));
    expect(replace.mock.calls.map((call) => call[2])).toContain("#/contacts");
  });

  it("leaves the record a reader moved on to before it saved", async () => {
    window.location.hash = "#/contacts/new";
    let finish: (created: { id: string }) => void = () => undefined;
    const created = new Promise<{ id: string }>((resolve) => {
      finish = resolve;
    });
    mount({
      listCreate: { open: true },
      create: () => created,
      drawnFor: "#/contacts/new",
    });
    fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    window.history.replaceState(null, "", "#/contacts/c9");
    await act(async () => finish({ id: "c1" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(window.location.hash).toBe("#/contacts/c9");
  });

  it("stays closed on the plain list", () => {
    window.location.hash = "#/contacts";
    mount({ listCreate: { open: false } });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("a create that is not a list's own", () => {
  it("leaves the address alone when cancelled", async () => {
    window.location.hash = "#/companies/acme";
    mount({});
    fireEvent.click(screen.getByRole("button", { name: "New contact" }));
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(window.location.hash).toBe("#/companies/acme");
  });

  it("leaves a reader who moved on before it saved where they are", async () => {
    window.location.hash = "#/companies/acme";
    let finish: (created: { id: string }) => void = () => undefined;
    const created = new Promise<{ id: string }>((resolve) => {
      finish = resolve;
    });
    mount({ create: () => created });
    fireEvent.click(screen.getByRole("button", { name: "New contact" }));
    fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    window.history.replaceState(null, "", "#/deals/d1");
    await act(async () => finish({ id: "c1" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(window.location.hash).toBe("#/deals/d1");
  });

  it("names no key on its button", () => {
    mount({});
    const button = screen.getByRole("button", { name: "New contact" });
    expect(button.getAttribute("aria-keyshortcuts")).toBeNull();
    expect(button.getAttribute("title")).toBeNull();
  });
});

describe("the list's create button", () => {
  it("names C as its key", () => {
    mount({ listCreate: { open: false } });
    const button = screen.getByRole("button", { name: "New contact" });
    expect(button.getAttribute("aria-keyshortcuts")).toBe("c");
    expect(button.getAttribute("title")).toBe("New contact · C");
  });

  it("names no key while single keys are off", () => {
    act(() => setSingleKeyShortcutsOn(false));
    mount({ listCreate: { open: false } });
    const button = screen.getByRole("button", { name: "New contact" });
    expect(button.getAttribute("aria-keyshortcuts")).toBeNull();
    expect(button.getAttribute("title")).toBeNull();
  });
});

describe("a list's create the seat may not take", () => {
  it("leaves its own address and opens no form", async () => {
    window.location.hash = "#/contacts/new";
    const client = new QueryClient();
    client.setQueryData(["me"], meFixture({ allow: { contact: ["read"] } }));
    mount({ listCreate: { open: true }, client });

    await waitFor(() => expect(window.location.hash).toBe("#/contacts"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens no form before /me answers, then leaves when it refuses", async () => {
    window.location.hash = "#/contacts/new";
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => undefined)),
    );
    const client = new QueryClient();
    mount({ listCreate: { open: true }, client });

    await screen.findByRole("button", { name: /New contact/ });
    expect(screen.queryByRole("dialog")).toBeNull();
    act(() => {
      client.setQueryData(["me"], meFixture({ allow: { contact: ["read"] } }));
    });
    await waitFor(() => expect(window.location.hash).toBe("#/contacts"));
    expect(screen.queryByRole("dialog")).toBeNull();
    vi.unstubAllGlobals();
  });

  it("says a read seat needs a full seat, though its role may create", async () => {
    const client = new QueryClient();
    client.setQueryData(
      ["me"],
      meFixture({ seat: "read", allow: { contact: ["read", "create"] } }),
    );
    mount({ client });

    const verb = await screen.findByRole("button", { name: /New contact/ });
    expect(verb.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(en["settings.seatRefused"])).toBeTruthy();
  });
});
