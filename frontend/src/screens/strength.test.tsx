/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { StrengthCard } from "./strength";

// The card's whole promise is "no mystery number": the composite score never
// renders alone, it carries the band that names it and the four factors that
// produce it. A card that showed 78 and nothing else would be asking the reader
// to trust an arithmetic they cannot see, which is the one thing this surface
// exists not to do.
//
// The factor rows had no test at all. They are the half of the promise that
// costs something to draw — four labels, four percentages, four meters — so
// they are the half that quietly stops being drawn.

type RelationshipStrength = components["schemas"]["RelationshipStrength"];

const strength: RelationshipStrength = {
  score: 78,
  bucket: "strong",
  factors: {
    recency: 0.9,
    frequency: 0.75,
    reciprocity: 0.6,
    direction: 0.5,
  },
  last_interaction: "2026-08-17T10:00:00Z",
  inbound_90d: 12,
  outbound_90d: 9,
  contributing_activity_ids: ["a-1", "a-2", "a-3"],
};

function mount(body: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      // `/me` decides native vs overlay, and overlay skips the read entirely —
      // an unstubbed probe would leave the card in its unavailable state and a
      // test could not tell that from a card that never rendered its rows.
      if (pathname.endsWith("/me")) {
        return jsonResponse({ system_of_record: { mode: "native" } });
      }
      return jsonResponse(body, status);
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <StrengthCard kind="person" id="p-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  cleanup();
});

describe("the relationship-strength card", () => {
  it("draws every factor behind the score, with its reading", async () => {
    mount(strength);

    expect(await screen.findByText("Strong")).toBeTruthy();
    expect(screen.getByText("Score 78/100")).toBeTruthy();
    // All four, each as a label and a percentage: three of four would be a
    // breakdown that does not add up to the number above it.
    for (const [label, reading] of [
      ["Recency", "90%"],
      ["Frequency", "75%"],
      ["Reciprocity", "60%"],
      ["Direction", "50%"],
    ]) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
      expect(screen.getByText(reading)).toBeTruthy();
    }
  });

  it("reads the honest zero when the response carries no factors", async () => {
    // A malformed answer degrades to the zero/none reading rather than taking
    // the record page down with it (craft T7): the contract guarantees these
    // fields, and one card is not the place to bet the page on that.
    mount({ score: 0, bucket: "none" });

    expect(await screen.findByText("Score 0/100")).toBeTruthy();
    expect(screen.getAllByText("0%")).toHaveLength(4);
  });

  it("says why there is no reading rather than showing an empty card", async () => {
    mount({ title: "Not found" }, 404);

    expect(await screen.findByText(/not found/i)).toBeTruthy();
  });
});
