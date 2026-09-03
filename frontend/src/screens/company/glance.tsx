// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { routeHash } from "../../app/router";
import { Avatar, Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelGroupHead } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { CommercialPanel, recordNamesIn } from "../company360";
import { CompanyContractState } from "../companycommercial";
import { CompanyProjects } from "../companyprojects";
import { peopleSlice } from "../companyrailshared";
import { CompanyRecentList } from "../companyrecent";
import type { CompanyTab } from "../companytab";
import { CompanyWorkCard } from "../companywork";
import "./glance.css";

type Organization360 = components["schemas"]["Organization360"];

// How many exchanges the fold opens. The 360 is a glance; the History tab is
// where the rest of the thread reads.
const THREAD_LIMIT = 6;

// How many people stand as chips before the remainder becomes one "+N" chip.
const CHIP_LIMIT = 3;

/**
 * The thread folded inside the 360: "Read the thread · N" opens the most
 * recent exchanges under the spine, and "Full history" is the History tab.
 *
 * It asks `sectionState` the same question the chronicle does, so a section
 * this reader may not see says so when opened rather than reading as an
 * account nobody has written to.
 */
export function ThreadFold({
  view,
  loading,
  onOpenHistory,
}: Readonly<{
  view?: Organization360;
  loading: boolean;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const logged = view?.activities?.data ?? [];
  const state = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    logged.length,
    loading,
  );
  const shown = Math.min(logged.length, THREAD_LIMIT);
  return (
    <>
      {open &&
        (state === "ready" ? (
          <CompanyRecentList
            activities={logged.slice(0, THREAD_LIMIT)}
            nameOf={recordNamesIn(view)}
          />
        ) : (
          <PanelBody>
            <SurfaceState
              state={state}
              emptyLabel={t("co.recent.empty")}
              emptyDetail={t("co.recent.emptyDetail")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        ))}
      <PanelBody className="co-360-foot">
        <Button
          small
          variant="ghost"
          aria-expanded={open}
          onClick={() => setOpen((current) => !current)}
        >
          {open
            ? t("co.360.hideThread")
            : t("co.360.readThread", { count: formatNumber(shown, locale) })}
        </Button>
        {onOpenHistory && (
          <Button small variant="ghost" onClick={onOpenHistory}>
            {t("co.360.fullHistory")}
          </Button>
        )}
      </PanelBody>
    </>
  );
}

/**
 * The money as one pane: what the account is under contract for and what it
 * has won and lost, then each open deal with its one status clause. The
 * contract block is the SAME component the Deals tab draws, so the two tabs
 * cannot say two things about one renewal.
 */
export function MoneyPane({
  organizationId,
  view,
  loading,
  readOnly,
  onAllDeals,
  onOpenRecord,
  verbs,
}: Readonly<{
  organizationId: string;
  view?: Organization360;
  loading: boolean;
  // An archived company joins no new project, so the group offers no verb
  // that would only be refused.
  readOnly: boolean;
  onAllDeals: () => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  verbs?: { deal?: ReactNode };
}>) {
  const t = useT();
  const present =
    Boolean(view?.deals) && !view?.sections_omitted.includes("deals");
  // The projects group's own state, read the way the deals group reads its
  // own: while the 360 is still arriving, or where this reader may not see
  // the projects, the group says so — an absent list handed to the links
  // section would draw "No projects yet" with an Attach verb over a section
  // that has not answered.
  const projects = view?.projects;
  const projectsState = sectionState(
    view,
    "projects",
    Boolean(projects),
    projects?.length ?? 0,
    loading,
  );
  return (
    <Panel
      title={t("co.commercial.title")}
      titleAction={
        present ? (
          <Button small variant="ghost" onClick={onAllDeals}>
            {t("co.commercial.allDeals")}
          </Button>
        ) : undefined
      }
    >
      <CommercialPanel
        view={view}
        extra={<CompanyContractState view={view} />}
        loading={loading}
        figuresOnly
      />
      <CompanyWorkCard
        view={view}
        loading={loading}
        onOpenRecord={onOpenRecord}
        bare
        verbs={verbs}
      />
      {/* The deliveries this company is part of — as the client, a partner or
          a subcontractor — as the group under the deals they came from. In
          this pane rather than one of its own: the money and the work it
          bought are one reading, and a third pane on the column read as a
          second page starting. */}
      {projectsState === "ready" || projectsState === "empty" ? (
        <CompanyProjects
          organizationId={organizationId}
          projects={projects}
          readOnly={readOnly}
          bare
        />
      ) : (
        <>
          <PanelGroupHead title={t("companyProjects.title")} level="h3" />
          <PanelBody>
            <SurfaceState
              state={projectsState}
              emptyLabel={t("projectLinks.emptyTitle")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        </>
      )}
    </Panel>
  );
}

/**
 * The account's people as chips: the first few by name, the rest as one
 * count, and the People tab behind the title. A glance at who is there, not
 * the roster — the roster has its own tab and the details column its top
 * three with their routes.
 */
export function PeopleChips({
  view,
  loading,
  onOpenTab,
}: Readonly<{
  view?: Organization360;
  loading: boolean;
  onOpenTab?: (tab: CompanyTab) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Past the server's cut `count` is absent, and both the "All" verb and the
  // remainder chip drop their figure with it.
  const { contacts, count, state } = peopleSlice(view, loading);
  const shown = contacts.slice(0, CHIP_LIMIT);
  const rest = contacts.length - shown.length;
  return (
    <Panel
      title={t("co.rail.people.title")}
      titleAction={
        state === "ready" && onOpenTab ? (
          <Button small variant="ghost" onClick={() => onOpenTab("people")}>
            {count != null
              ? t("co.rail.all", { count: formatNumber(count, locale) })
              : t("co.rail.allUncounted")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {state === "ready" ? (
          <ul className="co-people-chips">
            {shown.map((contact) => (
              <li key={contact.person_id}>
                <a
                  className="co-person-chip"
                  href={routeHash({
                    screen: "contacts",
                    id: contact.person_id,
                  })}
                >
                  <Avatar name={contact.full_name} size="xs" />
                  {contact.full_name}
                </a>
              </li>
            ))}
            {(rest > 0 || count == null) && (
              <li className="co-person-chip co-person-more">
                {count != null
                  ? `+${formatNumber(rest, locale)}`
                  : t("co.rail.more")}
              </li>
            )}
          </ul>
        ) : (
          <SurfaceState state={state} emptyLabel={t("co.rail.people.empty")}>
            {null}
          </SurfaceState>
        )}
      </PanelBody>
    </Panel>
  );
}
