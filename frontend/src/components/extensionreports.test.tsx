/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "../testing/story-utils";
import { ExtensionReports } from "./extensionreports";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

function draw(data: unknown[]) {
  globalThis.fetch = (async () => jsonResponse({ data })) as typeof fetch;
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ExtensionReports />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const base = {
  unit: "demo",
  population: "ext:demo:visit",
  excluded_by_permission: null,
};

describe("ExtensionReports", () => {
  it("draws a count and a breakdown, and says what the floor withheld", async () => {
    draw([
      {
        ...base,
        name: "ext:demo:visits",
        label: "Visits",
        kind: "count",
        withheld: false,
        total: 13,
        groups: [{ key: null, count: 13, withheld: false }],
      },
      {
        ...base,
        name: "ext:demo:by_site",
        label: "Visits by site",
        kind: "breakdown",
        by: "site",
        withheld: true,
        total: null,
        groups: [
          { key: "north", count: 9, withheld: false },
          { key: null, count: null, withheld: true },
        ],
      },
    ]);
    expect(await screen.findByText("Visits")).toBeInTheDocument();
    expect(screen.getByText("13")).toBeInTheDocument();
    expect(screen.getByText("north")).toBeInTheDocument();
    expect(
      screen.getByText("Withheld: too few records to show"),
    ).toBeInTheDocument();
  });

  it("names the records a field permission hides", async () => {
    draw([
      {
        ...base,
        name: "ext:demo:visits",
        label: "Visits",
        kind: "count",
        withheld: false,
        total: 0,
        excluded_by_permission: 13,
        groups: [{ key: null, count: 0, withheld: false }],
      },
    ]);
    expect(
      await screen.findByText(
        "13 records are hidden from you by a field permission.",
      ),
    ).toBeInTheDocument();
  });

  it("draws nothing when no extension registers a report", async () => {
    draw([]);
    await Promise.resolve();
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });
});
