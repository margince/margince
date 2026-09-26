/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  AnswerTable,
  type ExplainSource,
  QuestionFailure,
} from "./analytics.questions.answer";
import {
  ANSWER,
  EXPLANATION,
  QUERY,
  refusal,
  STAGES,
  USERS,
  WITHHELD_ANSWER,
} from "./analytics.questions.testkit";
import type {
  AnalyticsAnswer,
  AnalyticsQuery,
} from "./analytics.questions.vocab";
import { render } from "./analytics.testkit";
import { ProblemError } from "./common";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
}

type Asked = { url: string; body: unknown };

function stubServer(explanation: unknown = EXPLANATION): Asked[] {
  const asked: Asked[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (url.includes("/explain")) {
        const body = request
          ? await request.json()
          : JSON.parse(String(init?.body));
        asked.push({ url, body });
        return jsonResponse(explanation);
      }
      if (url.includes("/stages")) return jsonResponse(STAGES);
      if (url.includes("/users")) return jsonResponse(USERS);
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
  return asked;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderAnswer(
  answer: AnalyticsAnswer = ANSWER,
  query: AnalyticsQuery = QUERY,
  source: ExplainSource = { kind: "query", query },
) {
  render(
    <AnswerTable
      query={query}
      answer={answer}
      baseCurrency="EUR"
      source={source}
    />,
  );
}

const bodyRows = () => screen.getAllByRole("row").slice(1);

describe("the answer table", () => {
  it("names stages, formats money in the row's currency and says an unset group is unset", async () => {
    stubServer();
    renderAnswer();
    expect(await screen.findByText("Qualified")).toBeTruthy();
    expect(screen.getByText("€862,000.00")).toBeTruthy();
    expect(within(bodyRows()[2]).getByText(/\$125,000\.00/)).toBeTruthy();
    expect(within(bodyRows()[2]).getByText("Not set")).toBeTruthy();
  });

  it("counts every withheld group in one closing row, never as zeros", async () => {
    stubServer();
    renderAnswer(WITHHELD_ANSWER);
    await screen.findByText("Qualified");
    const rows = bodyRows();
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain("Qualified");
    expect(rows[1].textContent).toContain("Proposal");
    const withheld = rows[2];
    expect(withheld.textContent).toBe("3 groups withheld");
    expect(screen.getByText("Some groups are withheld")).toBeTruthy();
    expect(
      within(withheld).queryByRole("button", { name: /Explain/ }),
    ).toBeNull();
  });

  it("says a group value in the words the product already uses", async () => {
    stubServer();
    renderAnswer(
      {
        ...ANSWER,
        columns: ["status", "count"],
        rows: [
          { status: "won", count: 4, _withheld: false },
          { status: "open", count: 9, _withheld: false },
        ],
      },
      { ...QUERY, group_by: ["status"], measures: [{ fn: "count" }] },
    );
    expect(await screen.findByText("Won")).toBeTruthy();
    // No screen names an open deal yet, so it keeps its wire word.
    expect(screen.getByText("open")).toBeTruthy();
  });

  it("gives no figure for native amounts summed across currencies", async () => {
    stubServer();
    renderAnswer(
      {
        ...ANSWER,
        columns: ["stage_id", "count", "sum_amount_minor"],
        rows: ANSWER.rows.map(({ currency: _dropped, ...row }) => row),
      },
      { ...QUERY, group_by: ["stage_id"] },
    );
    await screen.findByText("Qualified");
    expect(screen.getAllByText("Mixed currencies")).toHaveLength(3);
    expect(
      screen.getByText("Amounts in different currencies are not added up"),
    ).toBeTruthy();
  });

  it("says only the first groups are shown when the answer reaches the limit", async () => {
    stubServer();
    renderAnswer(ANSWER, { ...QUERY, limit: 3 });
    expect(
      await screen.findByText(
        "Only the first 3 groups are shown. Add a filter to narrow the question.",
      ),
    ).toBeTruthy();
  });

  it("says there is nothing when no record matches", () => {
    stubServer();
    renderAnswer({ ...ANSWER, rows: [] });
    expect(screen.getByText("No records match this question.")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});

describe("a row's drill-down", () => {
  it("asks for the cell by its group keys in the question's order, null included", async () => {
    const user = userEvent.setup();
    const asked = stubServer();
    renderAnswer();
    await user.click(
      await screen.findByRole("button", { name: "Explain Not set, USD" }),
    );
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("€42,000.00");
    expect(asked).toEqual([
      {
        url: expect.stringContaining("/analytics/explain"),
        body: { query: QUERY, group: [null, "USD"] },
      },
    ]);
  });

  it("names records from the answer itself, reading nothing per row", async () => {
    const user = userEvent.setup();
    stubServer();
    const fetched = vi.mocked(fetch);
    renderAnswer();
    await user.click(
      await screen.findByRole("button", { name: "Explain Qualified, EUR" }),
    );
    const dialog = await screen.findByRole("dialog");
    await within(dialog).findByText("Fleet retrofit");
    const headers = within(dialog)
      .getAllByRole("columnheader")
      .map((cell) => cell.textContent);
    expect(headers[0]).toBe("Record");
    expect(headers).not.toContain("ID");
    const recordReads = fetched.mock.calls.filter(([input]) =>
      /\/(deals|leads|projects)\//.test(
        String(input instanceof Request ? input.url : input),
      ),
    );
    expect(recordReads).toEqual([]);
  });

  it("keeps a short id for a record the reader may not name", async () => {
    const user = userEvent.setup();
    stubServer({
      ...EXPLANATION,
      rows: [
        EXPLANATION.rows[0],
        { ...EXPLANATION.rows[1], id: "0199aa11-2222", label: undefined },
      ],
    });
    renderAnswer();
    await user.click(
      await screen.findByRole("button", { name: "Explain Qualified, EUR" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("0199aa11…")).toBeTruthy();
    expect(
      within(dialog).getByRole("columnheader", { name: "ID" }),
    ).toBeTruthy();
  });

  it("reads a saved run's cell through the run, never the question", async () => {
    const user = userEvent.setup();
    const asked = stubServer();
    renderAnswer(ANSWER, QUERY, { kind: "run", runId: "run-1" });
    await user.click(
      await screen.findByRole("button", { name: "Explain Qualified, EUR" }),
    );
    await screen.findByRole("dialog");
    expect(asked[0]).toEqual({
      url: expect.stringContaining("/analytics/runs/run-1/cells/explain"),
      body: { group: ["s-qual", "EUR"] },
    });
  });

  it("says a withheld cell is withheld and a capped one is capped", async () => {
    const user = userEvent.setup();
    stubServer({ ...EXPLANATION, withheld: true, rows: [] });
    renderAnswer();
    await user.click(
      await screen.findByRole("button", { name: "Explain Qualified, EUR" }),
    );
    expect(
      await screen.findByText(
        "This group covers too few records to show, so its records stay hidden.",
      ),
    ).toBeTruthy();
    cleanup();

    stubServer({ ...EXPLANATION, truncated: true });
    renderAnswer();
    await user.click(
      await screen.findByRole("button", { name: "Explain Qualified, EUR" }),
    );
    expect(
      await screen.findByText(
        "Showing the first 2 records. The group holds more.",
      ),
    ).toBeTruthy();
  });

  it("closes on Escape and hands focus back to the row", async () => {
    const user = userEvent.setup();
    stubServer();
    renderAnswer();
    const trigger = await screen.findByRole("button", {
      name: "Explain Qualified, EUR",
    });
    await user.click(trigger);
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await vi.waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("a question that was not answered", () => {
  it("leads a refusal with the suggestion, as guidance rather than an error", () => {
    render(
      <QuestionFailure
        error={
          new ProblemError(
            refusal(
              "privacy",
              "every group would describe fewer than five records",
              "group by stage_id alone",
            ),
          )
        }
      />,
    );
    const note = screen.getByRole("status");
    expect(note.textContent).toContain("group by stage_id alone");
    expect(note.textContent).toContain(
      "The answer would describe too few records.",
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("reads each refusal kind in its own words", () => {
    for (const [kind, words] of [
      ["invalid", "This question cannot be answered as asked."],
      ["unsupported", "This question is not supported yet."],
    ]) {
      render(
        <QuestionFailure
          error={new ProblemError(refusal(kind, "message", "try this"))}
        />,
      );
      expect(screen.getByRole("status").textContent).toContain(words);
      cleanup();
    }
  });

  it("falls back to the permission line for a 403", () => {
    render(
      <QuestionFailure
        error={
          new ProblemError({
            status: 403,
            code: "permission_denied",
            detail: "scope outside your lens",
          })
        }
      />,
    );
    expect(screen.getByRole("alert").textContent).not.toContain("lens");
  });
});
