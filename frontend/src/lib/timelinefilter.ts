// The cuts a record's History is read in, shared by the pages that draw the
// pills and the chronology that answers them.

// `conversations` is in the vocabulary but not in the base row. A page offers
// it through `ChronologyFilter`'s own flag once it renders the cut. A pill
// whose press changes nothing reads as a broken row.
export const TIMELINE_FILTERS = ["all", "changes"] as const;
export type TimelineFilter =
  | (typeof TIMELINE_FILTERS)[number]
  | "conversations";
