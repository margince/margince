// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  fixtureSection,
  newClient,
  render,
  renderWith,
  stubPhoneViewport,
} from "../testing/shellharness";
import { PageTitle } from "./shelltitle";

// One composed unit, so the unit's branch has a unit to name.
vi.mock("@composition/extensions", () => ({
  extensions: [{ name: "notes", verbs: [] }],
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A contact id in the form the server reads.
const ANNA_ID = "0a7e5c1d-6b2f-4c8e-9d3a-1f2e3d4c5b6a";

// The browser tab reads the page's path from the page back, so two tabs, the
// history list and a bookmark name different pages differently.
describe("the browser tab title", () => {
  it("names a section entry before its section and the product", () => {
    render(
      <PageTitle
        route={{ screen: "settings", id: "account" }}
        section={fixtureSection("account")}
      />,
    );
    expect(document.title).toBe("Account · Settings · Margince");
  });

  it("names a record by its name once it has been read", () => {
    const client = newClient();
    client.setQueryData(["contact", "ref", ANNA_ID], "Anna Weber");
    renderWith(
      client,
      <PageTitle route={{ screen: "contacts", id: ANNA_ID }} />,
    );
    expect(document.title).toBe("Anna Weber · Contacts · Margince");
  });

  it("names the list while a record's name is still on its way", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => undefined)),
    );
    render(<PageTitle route={{ screen: "contacts", id: ANNA_ID }} />);
    expect(document.title).toBe("Contacts · Margince");
  });

  it("names a record on a phone as it does on a desktop", () => {
    stubPhoneViewport();
    const client = newClient();
    client.setQueryData(["contact", "ref", ANNA_ID], "Anna Weber");
    renderWith(
      client,
      <PageTitle route={{ screen: "contacts", id: ANNA_ID }} />,
    );
    expect(document.title).toBe("Anna Weber · Contacts · Margince");
  });

  it("names a composed unit before Settings", () => {
    render(<PageTitle route={{ screen: "ext", id: "notes" }} />);
    expect(document.title).toBe("Notes · Settings · Margince");
  });

  it("names the Partners page before Companies, the list it belongs to", () => {
    render(<PageTitle route={{ screen: "partners" }} />);
    expect(document.title).toBe("Partners · Companies · Margince");
  });

  it("gives the title back when the page goes away", () => {
    document.title = "Margince";
    const { unmount } = render(<PageTitle route={{ screen: "deals" }} />);
    expect(document.title).toBe("Deals · Margince");
    unmount();
    expect(document.title).toBe("Margince");
  });
});
