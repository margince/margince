// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { CompanyFactsPanel } from "./companyfactspanel";

// Stating a fact and taking one away, from the reader's side of the screen.
// What these hold is the two things the panel exists for and one thing it must
// not do: every stored row is drawn (so a removal cannot uncover a row nobody
// saw), and a reader who may not write is refused once rather than shown verbs
// their save would reject.

type FactSeed = Readonly<{
  field: string;
  category: string;
  value: string;
  valueKey?: string;
  version?: number;
}>;

function fact(seed: FactSeed) {
  return {
    id: `f-${seed.field}-${seed.valueKey ?? ""}`,
    category: seed.category,
    field: seed.field,
    value: seed.value,
    value_key: seed.valueKey ?? "",
    source: "site_read",
    captured_by: "agent:deepread",
    updated_at: "2026-07-01T00:00:00Z",
    version: seed.version ?? 3,
  };
}

function stub(facts: readonly unknown[]) {
  const calls: {
    method: string;
    url: string;
    body: unknown;
    ifMatch: string | null;
  }[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    // A DELETE carries no body, and asking one for JSON throws before the
    // stub can answer — which reads exactly like the code failing to send the
    // request at all.
    const body =
      request.method === "POST" ? await request.clone().json() : undefined;
    calls.push({
      method: request.method,
      url: request.url,
      body,
      ifMatch: request.headers.get("If-Match"),
    });
    if (request.method === "GET") {
      return new Response(JSON.stringify({ data: facts }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }
    if (request.method === "POST") {
      return new Response(JSON.stringify({}), {
        status: 201,
        headers: { "content-type": "application/json" },
      });
    }
    // 204 exactly as the server sends it: no body, and no content-type
    // claiming there is one. With a JSON header the client parses an empty
    // string and the removal fails inside the test rather than in the code.
    return new Response(null, { status: 204 });
  });
  vi.stubGlobal("fetch", fetchMock);
  return calls;
}

function mount(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the facts a person can state and take away", () => {
  it("draws every stored row a reader may remove, past the preview cap", async () => {
    // Two rules could each uncover a row after a delete, and both have to be
    // off where the verb is offered: the COLLAPSE (these two spellings of one
    // offering are one row to groupFacts) and the PREVIEW CAP (five rows a
    // category). Seven rows in one category, two of them colliding, exercises
    // both — with five or fewer the cap could stay on unnoticed.
    stub([
      fact({
        category: "offering",
        field: "product",
        value: "Fleet Manager — telematics",
        valueKey: "fleet manager",
      }),
      fact({
        category: "offering",
        field: "service",
        value: "Fleet Manager rollout",
        valueKey: "fleet manager",
      }),
      ...["Depot", "Routing", "Telematics", "Fuel", "Driver"].map((one) =>
        fact({
          category: "offering",
          field: "capability",
          value: one,
          valueKey: one.toLowerCase(),
        }),
      ),
    ]);
    mount(<CompanyFactsPanel orgId="o-1" canEdit />);

    expect(await screen.findByText("Fleet Manager — telematics")).toBeTruthy();
    // Counted rather than sampled: an assertion on one row passes whenever the
    // cap happens to keep that row, which is the accident this rules out.
    const drawn = [
      "Fleet Manager — telematics",
      "Fleet Manager rollout",
      "Depot",
      "Routing",
      "Telematics",
      "Fuel",
      "Driver",
    ].filter((one) => screen.queryByText(one) !== null);
    expect(drawn).toHaveLength(7);
    expect(screen.queryByRole("button", { name: /Show all/ })).toBeNull();
  });

  it("removes a fact through the endpoint, with the version it was shown", async () => {
    const user = userEvent.setup();
    const calls = stub([
      fact({
        category: "signal",
        field: "certification",
        value: "ISO 9001",
        valueKey: "iso 9001",
        version: 7,
      }),
    ]);
    mount(<CompanyFactsPanel orgId="o-1" canEdit />);

    await user.click(await screen.findByRole("button", { name: /Remove ISO/ }));
    const dialog = await screen.findByRole("dialog");

    await user.click(within(dialog).getByRole("button", { name: "Remove" }));

    await waitFor(() =>
      expect(calls.some((one) => one.method === "DELETE")).toBe(true),
    );
    const removal = calls.find((one) => one.method === "DELETE");
    // The precondition, not just the address: without If-Match the removal is
    // last-write-wins and would delete a row somebody has since corrected.
    expect(removal?.ifMatch).toBe("7");
    // The fact key addresses the row; the version is what stops this removal
    // landing on a row somebody else has since corrected.
    // The whole key, not just the field: a multi-value fact has several rows
    // under one field, and a URL carrying only "certification" would name none
    // of them in particular.
    expect(decodeURIComponent(removal?.url ?? "")).toContain(
      "certification:iso 9001",
    );
  });

  it("states a new fact with the category its field belongs to", async () => {
    const user = userEvent.setup();
    const calls = stub([]);
    mount(<CompanyFactsPanel orgId="o-1" canEdit />);

    await user.click(await screen.findByRole("button", { name: "Add fact" }));
    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByRole("option", { name: "Customer" }));
    await user.type(screen.getByLabelText("What it says"), "Kaufhaus Norde");
    await user.click(screen.getByRole("button", { name: "Save fact" }));

    await waitFor(() =>
      expect(calls.some((one) => one.method === "POST")).toBe(true),
    );
    // The category rides with the field rather than being asked for
    // separately: a reader knows they are naming a customer, not that a
    // customer is a "signal".
    expect(calls.find((one) => one.method === "POST")?.body).toEqual({
      category: "signal",
      field: "named_customer",
      value: "Kaufhaus Norde",
    });
  });

  it("refuses a reader who may not write, once, and issues no request", async () => {
    const user = userEvent.setup();
    const calls = stub([
      fact({
        category: "company",
        field: "founded_year",
        value: "1998",
      }),
    ]);
    mount(<CompanyFactsPanel orgId="o-1" canEdit={false} reasonId="why" />);

    expect(await screen.findByText("1998")).toBeTruthy();
    // No remove control at all: a viewer who may not change a set is not a
    // viewer whose controls are temporarily unavailable.
    expect(screen.queryByRole("button", { name: /Remove/ })).toBeNull();

    const add = screen.getByRole("button", { name: "Add fact" });
    expect(add.getAttribute("aria-describedby")).toBe("why");
    await user.click(add);
    expect(screen.queryByLabelText("What it says")).toBeNull();
    expect(calls.every((one) => one.method === "GET")).toBe(true);
  });
});
