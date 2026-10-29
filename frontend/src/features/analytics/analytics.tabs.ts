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
import { useUrlParams } from "../../lib/urlstate";
import { useDataCoverage } from "./analytics.coverage";
import {
  useReportingCatalog,
  useReportingFramework,
} from "./reporting.queries";
import { readableViews, VIEW_SHAPE, viewOfSection } from "./reporting.views";

type ScopeKind =
  components["schemas"]["AnalyticsContext"]["default_scope"]["kind"];

/**
 * The open section, the tab row, and the menus beside it.
 *
 * Which section is open is an ADDRESS, so a reader can link to one. Back then
 * steps between the sections they looked at.
 * `landing` is set while the bare address waits on the framework to know its
 * tab. The screen draws that wait instead of a body.
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
  // A team set up for SDRs lands on their outcomes; everyone else on Sales.
  const framework = useReportingFramework(readsViews);
  const catalog = useReportingCatalog(readsViews);
  const coverageProbe = useDataCoverage();
  const requested = sectionFromAddress(
    segment,
    framework.data?.definition.template === "sdr" ? "sdr" : "performance",
    params.get("view"),
  );
  const views = readsViews ? viewTabs(catalog.data?.metrics) : [];
  const available = SECTIONS.filter((candidate) => {
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
  const choosingLanding =
    readsViews &&
    segment === undefined &&
    !params.get("view") &&
    framework.isPending;
  return {
    route,
    section: landOn(requested, available, {
      views,
      resolvingViews: readsViews && catalog.isPending,
    }),
    tabs: analyticsTabs(available),
    setup: available.filter(isSetupSection),
    library: libraryOf(available),
    landing: choosingLanding ? framework : undefined,
  };
}

// The tab row: every analysis section this reader may open, drawn whichever
// one is open, so the row never changes shape under the reader. The saved
// report library and Setup sit at the row's end as menus: they hold pages
// about reports, not a reading of the business.
const TAB_SECTIONS: readonly Section[] = [
  "performance",
  "sdr",
  "leads",
  "partners",
  "forecast",
  "pipeline",
  "outcomes",
  "delivery",
];

// The Performance views this catalog lets the seat read, as tabs. The first
// tab stands alone until the catalog answers, and for a seat that reads none.
// Its page then says which of the two it is.
function viewTabs(
  metrics: Parameters<typeof readableViews>[0] | undefined,
): Section[] {
  const views = metrics ? readableViews(metrics) : [];
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
  if (viewOfSection(requested)) {
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
type SetupSection = Extract<Section, "targets" | "definitions" | "coverage">;

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
