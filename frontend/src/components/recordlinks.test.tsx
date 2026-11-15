// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { mostSpecificFirst, RecordLinks } from "./recordlinks";

test("the most specific record leads, as the Worklist names it", () => {
  const filed = mostSpecificFirst([
    { entity_type: "company", entity_id: "co-1" },
    { entity_type: "contact", entity_id: "ct-1" },
    { entity_type: "project", entity_id: "pr-1" },
    { entity_type: "deal", entity_id: "de-1" },
    { entity_type: "lead", entity_id: "le-1" },
  ]);
  expect(filed.map((link) => link.entity_type)).toEqual([
    "lead",
    "deal",
    "project",
    "contact",
    "company",
  ]);
});

test("two records of one kind keep the order they arrived in", () => {
  const filed = mostSpecificFirst([
    { entity_type: "contact", entity_id: "ct-2" },
    { entity_type: "deal", entity_id: "de-1" },
    { entity_type: "contact", entity_id: "ct-1" },
  ]);
  expect(filed.map((link) => link.entity_id)).toEqual(["de-1", "ct-2", "ct-1"]);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

test("activity links resolve through one bounded request per populated kind", async () => {
  const requested: URL[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = new URL(new Request(input, init).url);
    requested.push(url);
    return Response.json({
      data: url.searchParams.getAll("id").map((id) => ({
        id,
        name: `Named ${id}`,
        archived: false,
      })),
    });
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["me"], meFixture({}));
  const view = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordLinks
          links={[
            { entity_type: "contact", entity_id: "ct-1" },
            { entity_type: "contact", entity_id: "ct-2" },
            { entity_type: "company", entity_id: "co-1" },
          ]}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(
    (await screen.findByRole("link", { name: "Named ct-1" })).getAttribute(
      "href",
    ),
  ).toContain("contacts/ct-1");
  expect(await screen.findByRole("link", { name: "Named ct-2" })).toBeTruthy();
  expect(await screen.findByRole("link", { name: "Named co-1" })).toBeTruthy();
  expect(requested).toHaveLength(2);
  expect(
    requested.every((url) => url.pathname === "/v1/records/references"),
  ).toBe(true);
  expect(
    requested
      .find((url) => url.searchParams.get("kind") === "contact")
      ?.searchParams.getAll("id"),
  ).toEqual(["ct-1", "ct-2"]);
  view.unmount();
  client.clear();
});
