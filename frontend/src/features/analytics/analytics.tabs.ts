// Which sections the Analytics tab row offers, and where an address lands when
// the reader may not open the one it names. Its own file so the screen keeps to
// drawing them.

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import {
  SECTIONS,
  type Section,
  sectionFromAddress,
} from "../../lib/analytics.address";
import { useCan } from "../../lib/capability";
import { useRoute } from "../../lib/router";
import { useMe } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { useDataCoverage } from "./analytics.coverage";
import { overviewViews } from "./analytics.overview.choice";
import { useReportingCatalog } from "./reporting.queries";
import { readableViews, VIEW_SHAPE, viewOfSection } from "./reporting.views";

type ScopeKind =
  components["schemas"]["AnalyticsContext"]["default_scope"]["kind"];

/**
 * The open section, the tab row, and the menus beside it.
 *
 * Which section is open is part of the address, so a reader can link to one. Back then
 * steps between the sections they looked at. A bare address opens Overview,
 * which chooses its own view, so no section waits on the framework to land.
 */
export function useAnalyticsSections(defaultScope: ScopeKind | undefined) {
  const route = useRoute();
  const [params] = useUrlParams();
  const segment = route.screen === "analytics" ? route.id : undefined;
  const can = {
    reports: useCan("report_definition", "read"),
    targets: useCan("sales_target", "read"),
    framework: useCan("reporting_framework", "read"),
    coverage: useCan("data_coverage", "read"),
  };
  const readsViews = can.reports && can.framework;
  // Until the seat's grants are known, every grant reads as refused, and the
  // bare address would land on Forecast and ask its figures first.
  const grantsKnown = useMe().data !== undefined;
  const catalog = useReportingCatalog(readsViews);
  const coverageProbe = useDataCoverage();
  const requested = sectionFromAddress(segment, "overview", params.get("view"));
  const views = readsViews ? viewTabs(catalog.data) : [];
  const opens = readsViews && opensOverview(catalog.data);
  const available = SECTIONS.filter((candidate) => {
    if (candidate === "overview") return opens;
    if (viewOfSection(candidate)) return views.includes(candidate);
    if (candidate === "reports") return can.reports;
    if (candidate === "targets") return can.targets;
    if (candidate === "definitions") return can.framework;
    if (candidate === "outcomes") return defaultScope === "owner";
    // The read starts only with the ops grant; the tab appears after the
    // server has answered.
    if (candidate === "coverage")
      return can.coverage && coverageProbe.isSuccess;
    return true;
  });
  const section = landOn(requested, available, {
    views,
    resolvingViews: !grantsKnown || (readsViews && catalog.isPending),
  });
  return {
    route,
    section,
    holding: !grantsKnown && !available.includes(section),
    tabs: analyticsTabs(available),
    setup: available.filter(isSetupSection),
    library: libraryOf(available),
  };
}

// Overview is offered while the catalog is on its way, so the bare address
// flashes no other tab. After that, it is offered when a preset is published.
function opensOverview(
  catalog: Parameters<typeof overviewViews>[0] | undefined,
): boolean {
  return catalog === undefined || overviewViews(catalog).length > 0;
}

// The tab row: every analysis section this reader may open, drawn whichever
// one is open, so the row never changes shape under the reader. The saved
// report library and Setup sit at the row's end as menus: they hold pages
// about reports, not a reading of the business.
const TAB_SECTIONS: readonly Section[] = [
  "overview",
  "performance",
  "sdr",
  "leads",
  "partners",
  "conversion",
  "forecast",
  "pipeline",
  "outcomes",
  "delivery",
];

// The Performance views this catalog lets the seat read, as tabs. The first
// tab stands alone until the catalog answers, and for a seat that reads none.
// Its page then says which of the two it is.
function viewTabs(
  catalog: Parameters<typeof readableViews>[0] | undefined,
): Section[] {
  const views = catalog ? readableViews(catalog) : [];
  return views.length > 0
    ? views.map((view) => VIEW_SHAPE[view].section)
    : ["performance"];
}

// The section an address opens when the reader may not open the one it names.
// A view this seat cannot read falls to the first one it can. A view is held
// while the catalog is still loading, so SDR outcomes never flashes Sales.
function landOn(
  requested: Section,
  available: readonly Section[],
  views: Readonly<{ views: readonly Section[]; resolvingViews: boolean }>,
): Section {
  if (available.includes(requested)) return requested;
  if (requested === "overview" || viewOfSection(requested)) {
    if (views.resolvingViews) return requested;
    return views.views[0] ?? "forecast";
  }
  return ["reports", "targets", "definitions"].includes(requested)
    ? "forecast"
    : requested;
}

// Where the Reports button leads: the saved library, or for a reader who may
// not read saved reports, their own custom questions.
function libraryOf(available: readonly Section[]): Section | undefined {
  if (available.includes("reports")) return "reports";
  return available.includes("questions") ? "questions" : undefined;
}

// Administration rather than analysis, behind one menu at the row's end.
export type SetupSection = Extract<
  Section,
  "targets" | "definitions" | "coverage"
>;

export const SETUP_LABEL: Readonly<Record<SetupSection, MessageKey>> = {
  targets: "reporting.targets",
  definitions: "reporting.definitions",
  coverage: "analytics.sectionCoverage",
};

function isSetupSection(section: Section): section is SetupSection {
  return section in SETUP_LABEL;
}

// The row keeps its own order, not the address list's that `available` follows.
export function analyticsTabs(available: readonly Section[]): Section[] {
  return TAB_SECTIONS.filter((section) => available.includes(section));
}
