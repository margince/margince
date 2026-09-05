/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CoverageExplorer } from "./coverageexplorer";

// The grid exists to answer "where are we thin" without becoming a contact ×
// every-colleague matrix, and to never let a blank cell mean two things.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubGraph(dropped = 0) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            nodes: [
              { id: "u-1", kind: "user", label: "Mira", root: false },
              { id: "p-1", kind: "person", label: "Dana Buyer", root: false },
              // No edge names Sam: an account contact nobody has written to is
              // the case the grid exists to show, and the graph is where the
              // account's contacts come from now.
              { id: "p-2", kind: "person", label: "Sam Silent", root: false },
            ],
            edges: [
              {
                from: "u-1",
                to: "p-1",
                kind: "in_contact_with",
                strength: 90,
                strength_bucket: "strong",
              },
            ],
            groups_omitted: [],
            dropped_count: dropped,
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
    ),
  );
}

function show(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// Takes the test's own session rather than reaching for the static API:
// userEvent.setup() is once per TEST, and a helper that sets up its own would
// hand each caller a second one.
async function open(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Compare coverage" }));
}

describe("comparing the colleagues a reader chooses", () => {
  it("reads a cell with no connection as Untried, not as a blank", async () => {
    const user = userEvent.setup();
    stubGraph();
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);

    // Sam Silent has no edge. "Untried" says nobody has written to them, which
    // is a different instruction from a cold band — and a blank cell says
    // neither, leaving the reader to guess which.
    expect(await screen.findByText("Sam Silent")).toBeTruthy();
    expect(screen.getByText("Untried")).toBeTruthy();
  });

  // §10.3: a table becomes structured cards before horizontal scrolling is the
  // only option. The narrow layout is CSS, but it can only work if every cell
  // carries the column header that says whose relationship it describes —
  // otherwise the meaning scrolls off the top with the header row.
  it("carries each column's colleague on the cell, so a narrow layout can label it", async () => {
    const user = userEvent.setup();
    stubGraph();
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);
    await screen.findByText("Sam Silent");

    // Queried from the document: the explorer opens in a dialog, which the
    // design system renders outside the caller's container.
    const labels = Array.from(document.querySelectorAll("td[data-label]")).map(
      (cell) => cell.getAttribute("data-label"),
    );
    expect(labels.length).toBeGreaterThan(0);
    // Every cell, not merely some: one unlabelled cell is one a phone reader
    // cannot attribute to anybody.
    expect(labels.every(Boolean)).toBe(true);
    expect(labels).toContain("Mira");
  });

  it("offers only colleagues who have actually reached this account", async () => {
    const user = userEvent.setup();
    stubGraph();
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);

    // A column the reader has to rule out is worse than no column, so a
    // colleague with no edge to this account never appears at all.
    expect(await screen.findByRole("button", { name: "Mira" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Nobody" })).toBeNull();
  });

  it("says a grid built from a capped read may be short", async () => {
    const user = userEvent.setup();
    stubGraph(7);
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);

    // "No connection" and "the read stopped short" are different claims, and a
    // reader told nobody covers a contact would stop looking.
    expect(await screen.findByText(/partial read/)).toBeTruthy();
  });

  it("filters the contact rows without touching the columns", async () => {
    const user = userEvent.setup();
    stubGraph();
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);
    await screen.findByText("Dana Buyer");

    await user.type(
      screen.getByRole("searchbox", { name: en["acctCoverage.findContact"] }),
      "Sam",
    );
    expect(screen.queryByText("Dana Buyer")).toBeNull();
    expect(screen.getByText("Sam Silent")).toBeTruthy();
  });

  // The matrix is one column per colleague, so it runs past the panel on any
  // real account. It had no scroll box at all: the columns past the right edge
  // were unreachable by mouse AND keyboard, and the `.coverage-scroll` rule
  // sat in the stylesheet referenced by nothing. `TableScroll` is the one box,
  // and its tab stop and announced name come with it.
  it("draws the matrix inside the shared scroll box", async () => {
    const user = userEvent.setup();
    stubGraph();
    show(<CoverageExplorer orgId="o-1" />);
    await open(user);
    await screen.findByText("Sam Silent");

    // Queried from the document: the explorer opens in a dialog, which the
    // design system renders outside the caller's container.
    const table = document.querySelector("table.coverage-table");
    expect(table).not.toBeNull();
    expect(table?.closest(".table-scroll")).not.toBeNull();
  });
});
