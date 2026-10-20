import type { components } from "../../api/schema";

type Section = components["schemas"]["ReportingSection"];
type Card = components["schemas"]["ReportingCard"];
type Selection = components["schemas"]["ReportingSelection"];
type Chart = components["schemas"]["ReportingChart"];
type BlockKind = Card["kind"];
type MetricId = components["schemas"]["ReportingMetricID"];
export type ReportingLayout = Section[];

// Mirrors oneChartKinds and unplacedKind in the reporting service's layout.go.
// The evaluation draws the first two once for the whole report. The page
// always draws the metric details table below the charts.
const ONE_CHART_KINDS: ReadonlySet<BlockKind> = new Set([
  "owner_attainment",
  "sdr_outcomes",
]);
const UNPLACED_KIND: BlockKind = "metric_reading";

/** The service's limits, so the editor offers no step it would refuse. */
export const MAX_SECTIONS = 8;
export const MAX_CARDS = 20;

/** A selection's layout, or one untitled section of full cards in block order. */
export function layoutOf(selection: Selection): ReportingLayout {
  if (selection.layout) return selection.layout;
  return [
    {
      cards: selection.blocks
        .filter((kind) => kind !== UNPLACED_KIND)
        .map((kind) => ({
          id: kind.replaceAll("_", "-"),
          kind,
          width: "full",
        })),
    },
  ];
}

/** Each kind once, where the layout first names it; the server derives the same. */
export function blocksOf(layout: ReportingLayout): BlockKind[] {
  return [
    ...new Set(layout.flatMap((section) => section.cards.map((c) => c.kind))),
  ];
}

/** Cards whose chart a metric still selected can draw; the rest are dropped. */
export function keptCards(
  layout: ReportingLayout,
  drawable: (card: Card) => boolean,
): ReportingLayout {
  return layout.map((section) => ({
    ...section,
    cards: section.cards.filter(drawable),
  }));
}

export function newCardId(layout: ReportingLayout): string {
  const taken = new Set(layout.flatMap((s) => s.cards.map((c) => c.id)));
  let n = taken.size + 1;
  while (taken.has(`card-${n}`)) n++;
  return `card-${n}`;
}

/** A chart a card may place: a kind for every metric, or one metric's. */
export type CardChoice = Readonly<{ kind: BlockKind; metric?: MetricId }>;

/**
 * What can still be placed: each allowed kind for all its metrics, and, where
 * two or more selected metrics share a kind, each metric's own. A choice that
 * would draw a chart already placed is left out, as the server refuses it.
 */
export function cardChoices(
  layout: ReportingLayout,
  metricBlocks: ReadonlyMap<MetricId, readonly BlockKind[]>,
): CardChoice[] {
  const placed = layout.flatMap((section) => section.cards);
  const choices: CardChoice[] = [];
  if (placed.length >= MAX_CARDS) return [];
  const kinds = [...new Set([...metricBlocks.values()].flat())].filter(
    (kind) => kind !== UNPLACED_KIND,
  );
  for (const kind of kinds) {
    const owners = [...metricBlocks].filter(([, blocks]) =>
      blocks.includes(kind),
    );
    const sameKind = placed.filter((card) => card.kind === kind);
    if (sameKind.length === 0) choices.push({ kind });
    if (
      owners.length < 2 ||
      ONE_CHART_KINDS.has(kind) ||
      sameKind.some((card) => !card.metric)
    )
      continue;
    for (const [metric] of owners) {
      if (!sameKind.some((card) => card.metric === metric))
        choices.push({ kind, metric });
    }
  }
  return choices;
}

/** One section as the page draws it: its title and the charts its cards place. */
export type LaidOutSection = Readonly<{
  title?: string;
  charts: readonly Chart[];
  full: readonly boolean[];
}>;

/** The evaluation's charts, grouped and sized by the layout's cards. */
export function laidOut(
  layout: ReportingLayout,
  charts: readonly Chart[],
): LaidOutSection[] {
  return layout
    .map((section) => {
      const placed = section.cards.flatMap((card) =>
        charts
          .filter(
            (chart) =>
              chart.kind === card.kind &&
              (!card.metric || chart.metric === card.metric),
          )
          .map((chart) => ({ chart, full: card.width === "full" })),
      );
      return {
        title: section.title?.trim() || undefined,
        charts: placed.map((item) => item.chart),
        full: placed.map((item) => item.full),
      };
    })
    .filter((section) => section.charts.length > 0);
}

const withSection = (
  layout: ReportingLayout,
  at: number,
  change: (section: Section) => Section,
): ReportingLayout =>
  layout.map((section, index) => (index === at ? change(section) : section));

/** The edits Edit report makes to a layout; each returns a new layout. */
export const layoutEdits = {
  renameSection: (layout: ReportingLayout, at: number, title: string) =>
    withSection(layout, at, (section) => ({ ...section, title })),
  addSection: (layout: ReportingLayout): ReportingLayout => [
    ...layout,
    { cards: [] },
  ],
  /** Its cards join the section before it, or after it when it is first. */
  removeSection(layout: ReportingLayout, at: number): ReportingLayout {
    if (layout.length < 2) return layout;
    const heir = at === 0 ? 1 : at - 1;
    const moved = layout[at]?.cards ?? [];
    return withSection(layout, heir, (section) => ({
      ...section,
      cards:
        at === 0 ? [...moved, ...section.cards] : [...section.cards, ...moved],
    })).filter((_, index) => index !== at);
  },
  moveCard(
    layout: ReportingLayout,
    at: number,
    card: number,
    step: -1 | 1,
  ): ReportingLayout {
    return withSection(layout, at, (section) => {
      const next = card + step;
      if (next < 0 || next >= section.cards.length) return section;
      const cards = [...section.cards];
      [cards[card], cards[next]] = [cards[next], cards[card]];
      return { ...section, cards };
    });
  },
  moveCardTo(
    layout: ReportingLayout,
    at: number,
    card: number,
    target: number,
  ): ReportingLayout {
    const moving = layout[at]?.cards[card];
    if (!moving || target === at || !layout[target]) return layout;
    const without = withSection(layout, at, (section) => ({
      ...section,
      cards: section.cards.filter((_, index) => index !== card),
    }));
    return withSection(without, target, (section) => ({
      ...section,
      cards: [...section.cards, moving],
    }));
  },
  setWidth: (
    layout: ReportingLayout,
    at: number,
    card: number,
    width: Card["width"],
  ) =>
    withSection(layout, at, (section) => ({
      ...section,
      cards: section.cards.map((c, index) =>
        index === card ? { ...c, width } : c,
      ),
    })),
  removeCard: (layout: ReportingLayout, at: number, card: number) =>
    withSection(layout, at, (section) => ({
      ...section,
      cards: section.cards.filter((_, index) => index !== card),
    })),
  /** A new card goes at the end of the last section, full width. */
  addCard(layout: ReportingLayout, choice: CardChoice): ReportingLayout {
    const sections = layout.length > 0 ? layout : [{ cards: [] }];
    const card: Card = {
      id: newCardId(sections),
      kind: choice.kind,
      width: "full",
    };
    if (choice.metric) card.metric = choice.metric;
    return withSection(sections, sections.length - 1, (section) => ({
      ...section,
      cards: [...section.cards, card],
    }));
  },
};
