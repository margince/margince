/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  boundedDecisions,
  decisionRow,
  readingsDay,
} from "../../testing/brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";

// A decisions lane read short draws a floor, unless the server states the
// exact total its store counted.

function draw(...args: Parameters<typeof readingsDay>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefReadingsStrip day={readingsDay(...args)} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function decisionsValue(): string | undefined {
  const card = screen
    .getByText(en["brief.readings.decisions"])
    .closest(".stat-card");
  return card?.querySelector(".stat-card-value")?.textContent ?? undefined;
}

afterEach(cleanup);

describe("the decisions reading over a short lane", () => {
  it("draws the server's exact decisions total rather than a floor", () => {
    draw(
      { review: 340, more_available: true },
      [decisionRow("a1", true)],
      [{ ...boundedDecisions(200, 1), total: 340 }],
    );
    expect(decisionsValue()).toBe("340");
  });

  it("keeps the floor when the server states no decisions total", () => {
    draw(
      { review: 180, more_available: true },
      [decisionRow("a1", true)],
      [boundedDecisions(200, 1)],
    );
    expect(decisionsValue()).toBe("180+");
  });
});

// Home draws the review reading, the field the Worklist strip draws, so the
// two cannot show different decisions figures.
describe("the decisions figure on Home", () => {
  it("is the review reading the Worklist strip draws", () => {
    draw(
      { review: 7 },
      [decisionRow("a1", true)],
      [{ ...boundedDecisions(1, 1), total: 9 }],
    );
    expect(decisionsValue()).toBe("7");
  });
});
