/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { readFileSync } from "node:fs";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { hasProgress, ImportProgressPanel } from "./import.progress";
import { run } from "./import.testkit";
import type { ImportRun } from "./importtypes";

afterEach(cleanup);

function Wrapper({ children }: Readonly<{ children: ReactNode }>) {
  return <LocaleProvider initial="en">{children}</LocaleProvider>;
}

function show(ui: ReactNode) {
  return render(ui, { wrapper: Wrapper });
}

const working: ImportRun = {
  ...(run as ImportRun),
  status: "running",
  phase: "importing",
  phase_object: "contact",
  started_at: "2026-08-13T10:00:00Z",
  elapsed_seconds: 205,
  idle_seconds: 2,
  heartbeat_age_seconds: 1,
  rate_rows_per_second: 35,
  progress: {
    objects: [
      { object: "company", rows_done: 500, rows_total: 500 },
      { object: "contact", rows_done: 740, rows_total: 6800 },
      { object: "links", rows_done: 0, rows_total: 6800 },
    ],
    counts: {
      created: 1200,
      updated: 40,
      unchanged: 0,
      skipped: 3,
      problems: 1,
    },
  },
};

function panel(next: ImportRun) {
  return <ImportProgressPanel run={next} waiting="Testing your file…" />;
}

describe("the import progress panel", () => {
  it("shows no Links bar until the links have a total", () => {
    show(
      panel({
        ...working,
        progress: {
          counts: working.progress?.counts ?? {
            created: 0,
            updated: 0,
            unchanged: 0,
            skipped: 0,
            problems: 0,
          },
          objects: [
            { object: "contact", rows_done: 10, rows_total: 100 },
            { object: "links", rows_done: 0, rows_total: 0 },
          ],
        },
      }),
    );
    expect(screen.getByRole("progressbar", { name: "Contacts" })).toBeVisible();
    expect(screen.queryByRole("progressbar", { name: "Links" })).toBeNull();
  });

  it("stretches its column so the strip and the bars take the part's width", () => {
    // jsdom cannot lay this out; the class and its rule are what keep them wide.
    const { container } = show(panel(working));
    expect(container.firstElementChild).toHaveClass("import--fill");
    const css = readFileSync("src/screens/import.css", "utf8");
    expect(css).toMatch(/\.import--fill\s*{[^}]*align-items:\s*stretch/);
  });

  it("reads the rows, the rate, the elapsed time and the counts", () => {
    show(
      panel({
        ...working,
        eta_scope: "object",
        eta_seconds_low: 130,
        eta_seconds_high: 170,
      }),
    );
    expect(
      screen.getByText(
        "1,240 of 14,100 rows · 35 rows/s · 3m 25s elapsed · about 3 min left in Contacts",
      ),
    ).toBeVisible();
    expect(
      screen.getByRole("progressbar", { name: "All files" }),
    ).toBeVisible();
    expect(
      screen.getByRole("progressbar", { name: "Contacts" }),
    ).toHaveAttribute("aria-valuetext", "Contacts: 740 of 6,800 rows");
    expect(
      screen.getByRole("region", { name: "Rows so far" }),
    ).toHaveTextContent("1,200");
  });

  it("hides the estimate until the server sends one", () => {
    show(panel(working));
    expect(screen.queryByText(/min left/)).toBeNull();
  });

  it("lowers the estimate only after two polls agree, and raises it at once", () => {
    const eta = (high: number, elapsed: number): ImportRun => ({
      ...working,
      elapsed_seconds: elapsed,
      eta_seconds_low: high - 40,
      eta_seconds_high: high,
    });
    const { rerender } = show(panel(eta(290, 10)));
    expect(screen.getByText(/about 5 min left/)).toBeVisible();
    rerender(panel(eta(170, 15)));
    expect(screen.getByText(/about 5 min left/)).toBeVisible();
    rerender(panel(eta(175, 20)));
    expect(screen.getByText(/about 3 min left/)).toBeVisible();
    rerender(panel(eta(400, 25)));
    expect(screen.getByText(/about 7 min left/)).toBeVisible();
  });

  it("shows stalled after 30 s idle, with the server's heartbeat age", () => {
    const { rerender } = show(panel({ ...working, idle_seconds: 45 }));
    expect(screen.getByRole("status")).toHaveTextContent("Still working on it");
    expect(screen.getByText(/No progress for 0m 45s/)).toBeVisible();
    rerender(
      panel({ ...working, idle_seconds: 45, heartbeat_age_seconds: 40 }),
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Waiting for the server",
    );
  });

  it("reads queued as waiting, not stalled", () => {
    const queued: ImportRun = {
      ...working,
      phase: undefined,
      started_at: undefined,
      progress: undefined,
      idle_seconds: 600,
    };
    show(panel(queued));
    expect(screen.getAllByText("Waiting to start").length).toBeGreaterThan(0);
    expect(screen.queryByText("Still working on it")).toBeNull();
  });

  it("announces a phase change once and no poll", () => {
    const { rerender } = show(panel(working));
    const region = screen.getByRole("status");
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(region).toHaveTextContent("Importing Contacts");
    rerender(
      panel({ ...working, elapsed_seconds: 210, rate_rows_per_second: 36 }),
    );
    expect(screen.getByRole("status")).toBe(region);
    expect(region).toHaveTextContent("Importing Contacts");
    rerender(panel({ ...working, phase: "linking", phase_object: "links" }));
    expect(region).toHaveTextContent("Linking records");
  });

  it("names no phase when the worker sends none, and the caller's line stands", () => {
    const bare: ImportRun = {
      ...(run as ImportRun),
      status: "validating",
      started_at: "2026-08-13T10:00:00Z",
    };
    expect(hasProgress(bare)).toBe(false);
    show(panel(bare));
    expect(screen.getAllByText("Testing your file…").length).toBeGreaterThan(0);
  });
});
