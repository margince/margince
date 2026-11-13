// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { render } from "./analytics.testkit";
import { sellerMarch, sellerMarchSummary } from "./progress.fixtures";
import type { ProgressAsk } from "./progress.queries";
import { ProgressSummary } from "./progress.summary";

afterEach(cleanup);

const ASK: ProgressAsk = {
  period: "month",
  year: 2026,
  month: 3,
  scope: { kind: "owner", id: sellerMarch.scope.id ?? "" },
};

function summaryStub(answer: () => Response): unknown[] {
  const asked: unknown[] = [];
  installFetchStub({
    "POST /analytics/progress/summary": (body: unknown) => {
      asked.push(body);
      return answer();
    },
  });
  return asked;
}

describe("ProgressSummary", () => {
  it("writes the summary from the page's figures and names who wrote it", async () => {
    const asked = summaryStub(() => jsonResponse(sellerMarchSummary));
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    expect(
      await screen.findByText(
        "You closed €60,000 of the €100,000 goal in March.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("That leaves €40,000 still missing to the goal."),
    ).toBeTruthy();
    expect(screen.getByText("Written by Margince")).toBeTruthy();
    expect(asked[0]).toMatchObject({
      period: "month",
      year: 2026,
      month: 3,
      scope_kind: "owner",
      refresh: false,
    });
  });

  it("asks the model again when the reader refreshes", async () => {
    const asked = summaryStub(() => jsonResponse(sellerMarchSummary));
    const user = userEvent.setup();
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    await user.click(
      await screen.findByRole("button", { name: "Refresh summary" }),
    );
    await waitFor(() => expect(asked).toHaveLength(2));
    expect(asked[1]).toMatchObject({ refresh: true });
  });

  it("draws nothing when no model wrote a sentence", async () => {
    const asked = summaryStub(() =>
      jsonResponse({
        ...sellerMarchSummary,
        generated_by: "deterministic",
        sentences: [],
        paragraphs: undefined,
      }),
    );
    const { container } = render(
      <ProgressSummary ask={ASK} progress={sellerMarch} />,
    );
    await waitFor(() => expect(asked).toHaveLength(1));
    await waitFor(() => expect(container.textContent).toBe(""));
  });

  it("draws nothing against a server that has no summary route", async () => {
    const asked = summaryStub(
      () =>
        new Response(JSON.stringify({ code: "not_found" }), { status: 404 }),
    );
    const { container } = render(
      <ProgressSummary ask={ASK} progress={sellerMarch} />,
    );
    await waitFor(() => expect(asked).toHaveLength(1));
    await waitFor(() => expect(container.textContent).toBe(""));
  });

  it("offers a retry when the summary could not be read", async () => {
    summaryStub(
      () => new Response(JSON.stringify({ code: "internal" }), { status: 500 }),
    );
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
