/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { StrengthPanel } from "./strength";

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
        <StrengthPanel kind="person" id="p-1" />
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

// The receipts behind the number, and the count that must not move with them.
describe("what the score was computed from", () => {
  const withReceipts: RelationshipStrength = {
    ...strength,
    contributing_activity_ids: ["a-1", "a-2", "a-3"],
    contributing_activities: [
      {
        activity_id: "a-1",
        kind: "email",
        subject: "Depot slot confirmed",
        occurred_at: "2026-09-01T09:00:00Z",
        content_state: "available",
      },
      {
        activity_id: "a-2",
        kind: "call",
        subject: "Rang about the retrofit",
        occurred_at: "2026-08-28T09:00:00Z",
        content_state: "available",
      },
    ],
  };

  function show(
    value: RelationshipStrength,
    onOpenEmail?: (id: string) => void,
  ) {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(value)),
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <StrengthPanel kind="person" id="p-1" onOpenEmail={onOpenEmail} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  }

  it("keeps the count from the ids, not from the rows it can name", async () => {
    // THREE activities fed the score and this reader may see two of them. The
    // number is a fact about the score; the list is a fact about the reader.
    // A count that shrank to the second would tell two readers different
    // things about one score.
    show(withReceipts);

    expect(await screen.findByText(/3 activities/)).toBeTruthy();
  });

  it("opens a cited message and leaves the other kinds as prose", async () => {
    const opened: string[] = [];
    show(withReceipts, (id) => opened.push(id));

    const email = await screen.findByRole("button", {
      name: /Depot slot confirmed/,
    });
    email.click();
    expect(opened).toEqual(["a-1"]);
    // The call is named and not pressable: it has no page of its own, and a
    // control that opened nothing would teach a reader the list does not work.
    expect(screen.getByText("Rang about the retrofit")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /Rang about the retrofit/ }),
    ).toBeNull();
  });

  it("says a message is there without saying what it said", async () => {
    show({
      ...withReceipts,
      contributing_activities: [
        {
          activity_id: "a-9",
          kind: "email",
          subject: null,
          occurred_at: "2026-09-01T09:00:00Z",
          content_state: "withheld",
        },
      ],
    });

    expect(await screen.findByText(/3 activities/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Depot slot/ })).toBeNull();
  });

  it("reads as it always did when the server names nothing", async () => {
    // Version skew, and the seat with no activity grant. Both send the ids and
    // no names.
    show({ ...withReceipts, contributing_activities: undefined });

    expect(await screen.findByText(/3 activities/)).toBeTruthy();
  });
});
