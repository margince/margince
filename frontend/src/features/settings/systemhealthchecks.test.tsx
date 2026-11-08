// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { attentionReport, healthReport } from "./systemhealth.fixtures";
import { HealthChecks, healthGroups } from "./systemhealthchecks";

type Source = components["schemas"]["HealthSource"];

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const withheld: Source = {
  id: "hidden",
  availability: "withheld",
  condition: "unknown",
  code: "withheld",
  scope: "workspace",
  permission: "job_health:read",
};

const ids = (sources: readonly Source[]) => sources.map((source) => source.id);

describe("healthGroups", () => {
  it("puts each check the reader may see in exactly one group", () => {
    const groups = healthGroups(
      [...healthReport.sources, withheld],
      healthReport.findings,
    );
    expect(ids(groups.setup)).toEqual(["recovery", "providers"]);
    expect(ids(groups.waiting)).toEqual(["jobs"]);
    expect(ids(groups.passing)).toEqual([
      "capture",
      "intake",
      "automatic",
      "search",
      "updates",
    ]);
  });

  it("leaves a check an open finding explains to that finding", () => {
    const explained = new Set(
      attentionReport.findings
        .filter((finding) => finding.requires_action)
        .map((finding) => finding.source),
    );
    expect(explained.size).toBeGreaterThan(0);
    const groups = healthGroups(
      attentionReport.sources,
      attentionReport.findings,
    );
    const grouped = [...groups.setup, ...groups.waiting, ...groups.passing];
    expect(grouped.filter((source) => explained.has(source.id))).toEqual([]);
  });
});

describe("HealthChecks", () => {
  function renderChecks() {
    render(
      <LocaleProvider initial="en">
        <HealthChecks
          groups={healthGroups(healthReport.sources, healthReport.findings)}
        />
      </LocaleProvider>,
    );
  }

  it("says what happened, why it matters and what to do for each check not set up", () => {
    renderChecks();
    const setup = screen.getByRole("list", { name: "Not set up yet" });
    const rows = within(setup).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    for (const row of rows) {
      for (const term of ["What happened", "Why it matters", "What to do"]) {
        expect(within(row).getByText(term)).toBeInTheDocument();
      }
    }
  });

  it("opens the page that sets a check up", async () => {
    renderChecks();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Open AI models" }));
    expect(window.location.hash).toBe("#/settings/models");
  });

  it("links a step that happens outside the app to its guide", () => {
    renderChecks();
    expect(
      screen.getByRole("link", { name: "Read the restore guide" }),
    ).toHaveAttribute("href", expect.stringContaining("run-a-restore-drill"));
  });

  it("lists the passing checks by name", () => {
    renderChecks();
    const passing = screen.getByRole("list", { name: "Passing" });
    expect(within(passing).getAllByRole("listitem")).toHaveLength(5);
  });
});
