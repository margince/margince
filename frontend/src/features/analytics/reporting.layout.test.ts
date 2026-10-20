import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  blocksOf,
  cardChoices,
  laidOut,
  layoutEdits,
  layoutOf,
  type ReportingLayout,
} from "./reporting.layout";

type Chart = components["schemas"]["ReportingChart"];

const chart = (kind: Chart["kind"], metric: Chart["metric"]) =>
  ({ kind, metric }) as Chart;

const twoSections: ReportingLayout = [
  {
    title: "Pipeline",
    cards: [
      { id: "a", kind: "stage_distribution", width: "half" },
      {
        id: "b",
        kind: "bookings_trend",
        metric: "bookings_won",
        width: "half",
      },
    ],
  },
  { cards: [{ id: "c", kind: "owner_attainment", width: "full" }] },
];

describe("a report's layout", () => {
  it("reads an old report as one section of full cards in block order", () => {
    const layout = layoutOf({
      blocks: ["bookings_trend", "stage_age"],
    } as components["schemas"]["ReportingSelection"]);
    expect(layout).toEqual([
      {
        cards: [
          { id: "bookings-trend", kind: "bookings_trend", width: "full" },
          { id: "stage-age", kind: "stage_age", width: "full" },
        ],
      },
    ]);
  });

  it("derives the blocks the server derives", () => {
    expect(blocksOf(twoSections)).toEqual([
      "stage_distribution",
      "bookings_trend",
      "owner_attainment",
    ]);
  });

  it("places each chart under its card, and a metric card takes only its own", () => {
    const sections = laidOut(twoSections, [
      chart("bookings_trend", "bookings_won"),
      chart("bookings_trend", "open_pipeline"),
      chart("stage_distribution", "open_pipeline"),
      chart("owner_attainment", "bookings_won"),
    ]);
    expect(sections).toEqual([
      {
        title: "Pipeline",
        charts: [
          chart("stage_distribution", "open_pipeline"),
          chart("bookings_trend", "bookings_won"),
        ],
        full: [false, false],
      },
      {
        title: undefined,
        charts: [chart("owner_attainment", "bookings_won")],
        full: [true],
      },
    ]);
  });

  it("offers each metric's chart where two metrics share a kind, and nothing already placed", () => {
    const metricBlocks = new Map<
      components["schemas"]["ReportingMetricID"],
      components["schemas"]["ReportingBlockKind"][]
    >([
      ["bookings_won", ["bookings_trend", "owner_attainment"]],
      ["open_pipeline", ["bookings_trend", "stage_distribution"]],
    ]);
    expect(cardChoices(twoSections, metricBlocks)).toEqual([
      { kind: "bookings_trend", metric: "open_pipeline" },
    ]);
  });

  it("offers no metric details card and no one-metric chart of a kind drawn once", () => {
    const metricBlocks = new Map<
      components["schemas"]["ReportingMetricID"],
      components["schemas"]["ReportingBlockKind"][]
    >([
      ["bookings_won", ["owner_attainment", "metric_reading"]],
      ["qualified_pipeline_created", ["owner_attainment"]],
    ]);
    expect(cardChoices([{ cards: [] }], metricBlocks)).toEqual([
      { kind: "owner_attainment" },
    ]);
  });

  it("offers nothing once twenty cards are placed", () => {
    const full: ReportingLayout = [
      {
        cards: Array.from({ length: 20 }, (_, n) => ({
          id: `card-${n}`,
          kind: "stage_age" as const,
          width: "half" as const,
        })),
      },
    ];
    const metricBlocks = new Map<
      components["schemas"]["ReportingMetricID"],
      components["schemas"]["ReportingBlockKind"][]
    >([["bookings_won", ["bookings_trend"]]]);
    expect(cardChoices(full, metricBlocks)).toEqual([]);
  });

  it("leaves the metric details table out of an old report's cards", () => {
    const layout = layoutOf({
      blocks: ["metric_reading", "stage_age"],
    } as components["schemas"]["ReportingSelection"]);
    expect(layout[0]?.cards.map((card) => card.kind)).toEqual(["stage_age"]);
  });

  it("moves a removed section's cards to the section before it", () => {
    expect(
      layoutEdits.removeSection(twoSections, 1)[0]?.cards.map((c) => c.id),
    ).toEqual(["a", "b", "c"]);
  });

  it("moves a card within its section and into another", () => {
    const swapped = layoutEdits.moveCard(twoSections, 0, 0, 1);
    expect(swapped[0]?.cards.map((c) => c.id)).toEqual(["b", "a"]);
    const moved = layoutEdits.moveCardTo(twoSections, 0, 1, 1);
    expect(moved.map((s) => s.cards.map((c) => c.id))).toEqual([
      ["a"],
      ["c", "b"],
    ]);
  });

  it("adds a new card to the last section with an id of its own", () => {
    const added = layoutEdits.addCard(twoSections, {
      kind: "bookings_trend",
      metric: "open_pipeline",
    });
    expect(added[1]?.cards.at(-1)).toEqual({
      id: "card-4",
      kind: "bookings_trend",
      metric: "open_pipeline",
      width: "full",
    });
  });
});
