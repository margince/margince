/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { type Locale, LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { TRACED_RUNS } from "./automations.fixtures";
import { RunRow } from "./automations.runrow";

type AutomationRun = components["schemas"]["AutomationRun"];

afterEach(cleanup);

function traced(id: string): AutomationRun {
  const found = TRACED_RUNS.find((one) => one.id === id);
  if (!found) throw new Error(`No traced run ${id}`);
  return found;
}

function draw(run: AutomationRun, locale: Locale = "en") {
  render(
    <LocaleProvider initial={locale}>
      <ul>
        <RunRow run={run} />
      </ul>
    </LocaleProvider>,
  );
  return screen.getByRole("listitem");
}

describe("a traced run", () => {
  it("says what it did to which record, linked to that record's page", () => {
    const row = draw(traced("r-task"));
    expect(row).toHaveTextContent(
      "Created task “Send the proposal” on Globex Renewal",
    );
    const link = within(row).getByRole("link", { name: "Globex Renewal" });
    expect(link.getAttribute("href")).toMatch(
      /#\/deals\/01a1e5f1-7a5c-7000-8000-00000000d001$/,
    );
    expect(row).toHaveTextContent("When Deal moved to stage Proposal");
    expect(row).not.toHaveTextContent(/applied create_task|entered Proposal/);
  });

  it("says what waits for approval", () => {
    expect(draw(traced("r-draft"))).toHaveTextContent(
      "Waiting for approval: a draft email on Globex Renewal",
    );
  });

  it("names a record the reader may not see by its kind, never its id", () => {
    const row = draw(traced("r-hidden"));
    expect(row).toHaveTextContent("Sent a notification about a deal");
    expect(within(row).queryByRole("link")).toBeNull();
    expect(row).not.toHaveTextContent("00000000d002");
  });

  it("reads in the reader's language", () => {
    expect(draw(traced("r-task"), "de")).toHaveTextContent(
      "Aufgabe „Send the proposal“ zu Globex Renewal erstellt",
    );
  });

  it("keeps its event id behind technical details", async () => {
    const user = userEvent.setup();
    const row = draw(traced("r-task"));
    const summary = within(row).getByText(englishCopy["auto.runs.technical"]);
    expect(summary.closest("details")).not.toHaveAttribute("open");
    await user.click(summary);
    expect(
      await within(row).findByText("01a1e5f1-7a5c-7000-8000-0000000000e1"),
    ).toBeVisible();
  });
});

describe("a run this client cannot word", () => {
  it("falls back to the server's lines for an action kind it predates", () => {
    const row = draw({
      ...traced("r-task"),
      action_kinds: ["create_task", "teleport"],
      action_result: "applied create_task, teleport",
    });
    expect(row).toHaveTextContent("Result applied create_task, teleport");
    expect(
      within(row).getByRole("link", { name: "Globex Renewal" }),
    ).toBeVisible();
    expect(row).not.toHaveTextContent("Created task");
  });

  it("falls back to the server's evidence for a trigger it predates", () => {
    const row = draw({
      ...traced("r-task"),
      trigger_event_type: "deal.teleported",
    });
    expect(row).toHaveTextContent(
      "Trigger deal Globex Renewal entered Proposal",
    );
    expect(row).not.toHaveTextContent("When");
  });

  it("shows an older server's three lines as they were", () => {
    const row = draw(traced("r-older"));
    expect(row).toHaveTextContent(
      "Trigger deal BÄR Pharma entered Negotiation",
    );
    expect(row).toHaveTextContent("Target deal:BÄR Pharma");
    expect(row).toHaveTextContent("Result created follow-up task");
    expect(
      within(row).queryByText(englishCopy["auto.runs.technical"]),
    ).toBeNull();
  });
});
