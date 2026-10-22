import type { components } from "../../api/schema";

// Kanban, table and deal detail share the fetched records and approval flow.
// Mixed-currency columns never sum native minor units; weighting stays server-side.

export { buildColumns, dealStatusTone } from "./deals.columns";
export { DealScreen, useDeal } from "./deals.detail";
export { DealDetails } from "./deals.details";
export { dealEditFields, usePartnerOptions } from "./deals.fields";
export { DealsScreen } from "./deals.list";
export { OffersPanel } from "./deals.overview";
export {
  buildStageTotals,
  dealsQueryParams,
  type StageTotals,
} from "./deals.queries";
export { mapDealCreate, mapDealUpdate } from "./deals.write";

export type Deal = components["schemas"]["Deal"];

export type Company = components["schemas"]["Company"];

export type Stage = components["schemas"]["Stage"];

export type Pipeline = components["schemas"]["Pipeline"];

export type Offer = components["schemas"]["Offer"];

// The FX-converted base-currency sub-line (D-14): shown only when the deal
// carries a frozen fx_rate_to_base (won/lost deals freeze it at close; open
// deals in a non-base currency may not have one yet). Its body moved into
// dealcockpit.tsx, where the money cell draws it as a second line; re-exported
// here so this stays the one import path this file's own stories and tests
// already use.
export { FxLine } from "./deal360/dealcockpit";
