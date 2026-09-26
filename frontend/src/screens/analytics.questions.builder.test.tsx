/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { QuestionBuilder } from "./analytics.questions.builder";
import { newDraft, type QuestionDraft } from "./analytics.questions.draft";
import { ENTITIES, STAGES, USERS } from "./analytics.questions.testkit";
import { render } from "./analytics.testkit";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/stages")) return jsonResponse(STAGES);
      if (url.includes("/users")) return jsonResponse(USERS);
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Harness({
  start,
  onAsk,
}: Readonly<{ start: QuestionDraft; onAsk: (draft: QuestionDraft) => void }>) {
  const [draft, setDraft] = useState(start);
  return (
    <QuestionBuilder
      entities={ENTITIES}
      draft={draft}
      onChange={setDraft}
      onAsk={() => onAsk(draft)}
      onSave={() => {}}
      asking={false}
      saving={false}
    />
  );
}

// The sentence a control's aria-describedby points at, as a reader hears it.
function description(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

function renderBuilder(start: QuestionDraft = newDraft("")) {
  const asked: QuestionDraft[] = [];
  render(<Harness start={start} onAsk={(draft) => asked.push(draft)} />);
  return asked;
}

describe("the question builder", () => {
  it("refuses to ask until a report is chosen, and says why", async () => {
    const user = userEvent.setup();
    const asked = renderBuilder();
    const ask = screen.getByRole("button", { name: "Ask" });
    expect(description(ask)).toBe("Choose a report to ask about.");
    await user.click(ask);
    expect(asked).toEqual([]);

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Report" }),
      "All deals by stage",
    );
    await user.click(screen.getByRole("button", { name: "Ask" }));
    expect(asked.map((draft) => draft.entity)).toEqual(["deals-by-stage"]);
  });

  it("offers count no field and narrows a sum to the report's measures", async () => {
    const user = userEvent.setup();
    renderBuilder(newDraft("deals-by-stage"));
    const row = screen.getByRole("listitem", { name: "Measure 1" });
    expect(
      within(row).queryByRole("combobox", { name: "Field, measure 1" }),
    ).toBeNull();

    await pickOption(
      user,
      within(row).getByRole("combobox", { name: "Calculation, measure 1" }),
      "Sum",
    );
    await user.click(
      within(row).getByRole("combobox", { name: "Field, measure 1" }),
    );
    const offered = (await screen.findAllByRole("option")).map(
      (option) => option.textContent,
    );
    expect(offered).toEqual(["Amount", "Weighted amount"]);
    expect(description(screen.getByRole("button", { name: "Ask" }))).toBe(
      "Choose a field for every measure.",
    );
  });

  it("draws a filter's value by the field's shape, and none for a null test", async () => {
    const user = userEvent.setup();
    renderBuilder(newDraft("deals-by-stage"));
    await user.click(screen.getByRole("button", { name: "Add filter" }));
    const row = screen.getByRole("listitem", { name: "Filter 1" });
    await pickOption(
      user,
      within(row).getByRole("combobox", { name: "Field, filter 1" }),
      "Win probability",
    );
    expect(
      within(row)
        .getByRole("textbox", { name: "Value, filter 1" })
        .getAttribute("inputmode"),
    ).toBe("decimal");

    await pickOption(
      user,
      within(row).getByRole("combobox", { name: "Operator, filter 1" }),
      "is empty",
    );
    expect(
      within(row).queryByRole("textbox", { name: "Value, filter 1" }),
    ).toBeNull();
  });

  it("takes a yes-or-no field as a two-way choice", async () => {
    const user = userEvent.setup();
    renderBuilder(newDraft("meeting-conversion"));
    await user.click(screen.getByRole("button", { name: "Add filter" }));
    const row = screen.getByRole("listitem", { name: "Filter 1" });
    await pickOption(
      user,
      within(row).getByRole("combobox", { name: "Field, filter 1" }),
      "Converted",
    );
    expect(
      within(row).getByRole("group", { name: "Value, filter 1" }),
    ).toBeTruthy();
  });

  it("drops a grouping the newly chosen report does not have", async () => {
    const user = userEvent.setup();
    renderBuilder({
      ...newDraft("deals-by-stage"),
      groupBy: ["stage_id", "status"],
    });
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Report" }),
      "Leads by status",
    );
    expect(screen.getByRole("combobox", { name: "Group by" }).textContent).toBe(
      "Status",
    );
  });

  it("groups in the order the picker lists, whatever order the picks came in", async () => {
    const user = userEvent.setup();
    const asked = renderBuilder(newDraft("deals-by-stage"));
    const groupBy = screen.getByRole("combobox", { name: "Group by" });
    await user.click(groupBy);
    await user.click(await screen.findByRole("option", { name: "Stage" }));
    await user.click(screen.getByRole("option", { name: "Currency" }));
    await user.keyboard("{Escape}");
    expect(groupBy.textContent).toBe("Currency, Stage");
    await user.click(screen.getByRole("button", { name: "Ask" }));
    expect(asked[0]?.groupBy).toEqual(["currency", "stage_id"]);
  });

  it("names each row's pickers by the row they sit in", async () => {
    const user = userEvent.setup();
    renderBuilder(newDraft("deals-by-stage"));
    await user.click(screen.getByRole("button", { name: "Add measure" }));
    expect(
      screen.getByRole("combobox", { name: "Calculation, measure 2" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Add filter" }));
    expect(
      screen.getByRole("combobox", { name: "Operator, filter 1" }),
    ).toBeTruthy();
  });
});
