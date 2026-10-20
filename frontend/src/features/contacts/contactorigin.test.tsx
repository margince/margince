// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RecordOriginFacts } from "../../design-system/recordfacts";
import { LocaleProvider } from "../../i18n";
import { archivedContact } from "./contactarchived.fixtures";

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

function show(source: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordOriginFacts record={{ ...archivedContact, source }} />
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
