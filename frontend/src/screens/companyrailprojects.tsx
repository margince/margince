import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import { Button, Disclosure } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
// The row shape this file draws (co-row-meta) is the record page's,
// defined in company360.css. Imported HERE for the same reason
// companyraildeals.tsx does: this file renders wherever it is mounted, and a
// sibling having loaded the stylesheet first is not something it can assume.
import "../design-system/recordcard.css";
import "./company360.css";
import {
  RAIL_ROW_LIMIT,
  SectionSummary,
  sectionAnswered,
} from "./companyrailshared";
import "./companyrailprojects.css";
import { PhaseBadge } from "./projects";
import type { ProjectPhase } from "./projects.form";

// The account's own projects at a glance. Beside DealsSection under "Active
// deals" in the rail: the account's work in flight is both its open deals and
// the deliveries it is part of, and a reader working down the column meets
// the second right after the first.

type Company360 = components["schemas"]["Company360"];
type Project = components["schemas"]["Company360Project"];

/**
 * ProjectsSection is the account's own projects at a glance: the top
 * RAIL_ROW_LIMIT rows, each naming the project and where it stands.
 * `view.projects` already arrives work-in-motion first (delivering, pursuing,
 * initiative, then closed: the 360's own contract), so nothing here sorts
 * the rows a second time.
 */
export function ProjectsSection({
  view,
  loading,
  onTab,
}: Readonly<{
  view?: Company360;
  loading: boolean;
  onTab: (tab: "deals") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const projects = view?.projects ?? [];
  const state = sectionState(
    view,
    "projects",
    Boolean(view?.projects),
    projects.length,
    loading,
  );
  const answered = sectionAnswered(state);
  // Whole only while the page is the whole set: `projects_page` is absent
  // exactly when `projects` is, so a section not yet answered reads no count
  // rather than a `has_more` off a page that never arrived.
  const count =
    answered && view?.projects_page && !view.projects_page.has_more
      ? projects.length
      : undefined;
  return (
    <Disclosure
      className="co-sect"
      open
      summary={
        <SectionSummary title={t("co.rail.projects.title")} count={count} />
      }
    >
      {state === "ready" ? (
        <ul className="record-card-list">
          {projects.slice(0, RAIL_ROW_LIMIT).map((project) => (
            <li key={project.project_id}>
              <ProjectRailRow project={project} />
            </li>
          ))}
        </ul>
      ) : (
        <PanelBody>
          <SurfaceState
            loadingLabel={t("co.rail.projects.title")}
            state={state}
            emptyLabel={t("co.rail.projects.empty")}
          >
            {null}
          </SurfaceState>
          {/* Inside the body, under the sentence it answers: the verb stands
              on the same margin as the words above it, the way the deals
              section's own empty verb does. Outside, it sat at the section's
              edge and read as chrome of the rail rather than as this
              section's one thing to do. */}
          {state === "empty" && (
            <div className="card-actions">
              <Button variant="ghost" onClick={() => onTab("deals")}>
                {t("co.rail.add")}
              </Button>
            </div>
          )}
        </PanelBody>
      )}
      {state === "ready" && (
        <div className="card-actions">
          <Button variant="ghost" onClick={() => onTab("deals")}>
            {count != null
              ? t("co.rail.all", { count: formatNumber(count, locale) })
              : t("co.rail.allUncounted")}
          </Button>
        </div>
      )}
    </Disclosure>
  );
}

// The same card face the deals rail row wears (design-system/recordcard.css):
// border, r-md, elevated background, hover. The whole card is the link, not a
// name inside it: a project row carries no other control to collide with.
function ProjectRailRow({ project }: Readonly<{ project: Project }>) {
  return (
    <a
      className="record-card co-project-card"
      href={routeHash({ screen: "projects", id: project.project_id })}
      // Named for the project alone: the card's other text (the owner, the
      // phase badge) would otherwise lead the link's computed name, the same
      // reason the deal rail row's own link carries an explicit label.
      aria-label={project.name}
    >
      <span className="record-card-name">{project.name}</span>
      <span className="co-project-phase">
        <PhaseBadge phase={project.phase as ProjectPhase} />
      </span>
      {project.owner_name && (
        <p className="co-row-meta t-caption co-project-meta">
          {project.owner_name}
        </p>
      )}
    </a>
  );
}
