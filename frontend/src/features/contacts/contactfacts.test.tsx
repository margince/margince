// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { archivedContact } from "./contactarchived.fixtures";
import { ContactFacts } from "./contactfacts";

type Contact = components["schemas"]["Contact"];

beforeEach(() => {
  vi.stubGlobal("fetch", () =>
    Promise.resolve(
      new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(source: string, contact: Contact = archivedContact) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactFacts contact={{ ...contact, source }} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a contact an import wrote", () => {
  it("an imported contact is labelled imported, not typed by a person", () => {
    show("mirror:csv:pipedrive-export:contact:p-9");

    expect(
      screen.getByText("Imported from pipedrive-export"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Typed by/)).toBeNull();
  });

  it("a typed contact still reads as typed", () => {
    show("manual");

    expect(screen.queryByText(/Imported/)).toBeNull();
  });
});

// The archived page draws the contact from its own row, so the row's
// `created_by` is all that can name whom an agent made it for.
describe("a contact an agent created", () => {
  const AGENT = "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69";

  it("names the colleague the agent worked for and the tool", () => {
    show("manual", {
      ...archivedContact,
      captured_by: AGENT,
      created_by: {
        actor_type: "agent",
        actor_id: AGENT,
        on_behalf_of_name: "Anna Weber",
        agent_client: "Claude",
      },
    });

    expect(
      screen.getByText("Made by Claude for Anna Weber"),
    ).toBeInTheDocument();
  });
});

describe("the ways to reach a contact in the header", () => {
  const stamp = { source: "manual", captured_by: "human:u-1" } as const;
  const reachable: Contact = {
    ...archivedContact,
    archived_at: null,
    primary_email: "dana@brandt.example",
    emails: [
      {
        id: "e-1",
        email: "front.desk@brandt.example",
        email_type: "work",
        is_primary: false,
        position: 0,
        ...stamp,
      },
      {
        id: "e-2",
        email: "dana@brandt.example",
        email_type: "work",
        is_primary: true,
        position: 1,
        ...stamp,
      },
    ],
    phones: [
      {
        id: "ph-1",
        phone: "+49 30 1111",
        phone_type: "work",
        is_primary: false,
        position: 0,
        ...stamp,
      },
      {
        id: "ph-2",
        phone: "+49 30 2222",
        phone_type: "mobile",
        is_primary: true,
        position: 1,
        ...stamp,
      },
    ],
  };

  it("shows the server's address and the primary number, not the first rows", () => {
    show("manual", reachable);

    expect(screen.getByText("dana@brandt.example")).toBeInTheDocument();
    expect(screen.queryByText("front.desk@brandt.example")).toBeNull();
    expect(
      screen.getByRole("link", { name: "+49 30 2222" }).getAttribute("href"),
    ).toBe("tel:+49302222");
    expect(screen.queryByText("+49 30 1111")).toBeNull();
  });

  it("shows no address when the server names none", () => {
    show("manual", { ...reachable, primary_email: null });

    expect(screen.queryByText(/@brandt\.example/)).toBeNull();
  });
});

describe("a contact whose country does not fit the postal code and city", () => {
  const berlin = { postal_code: "10115", city: "Berlin", country: "AT" };

  it("flags the city with the country they resolve to", () => {
    show("manual", {
      ...archivedContact,
      address: berlin,
      address_country_mismatch: { resolved_country: "DE" },
    });

    expect(screen.getByText("Berlin")).toBeInTheDocument();
    expect(
      screen.getByText("Postal code and city are in Germany"),
    ).toBeInTheDocument();
  });

  it("shows no flag when the server reports no mismatch", () => {
    show("manual", { ...archivedContact, address: berlin });

    expect(screen.getByText("Berlin")).toBeInTheDocument();
    expect(screen.queryByText(/Postal code and city are in/)).toBeNull();
  });
});
