// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { routeHash } from "../../app/router";
import { Avatar, Button, Disclosure } from "../../design-system/atoms";
import { Panel, PanelBody, PanelGroupHead } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatDateAbbrev, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { CommercialPanel, recordNamesIn } from "../company360";
import { CompanyContractState } from "../companycommercial";
import { CompanyProjects } from "../companyprojects";
import { peopleSlice } from "../companyrailshared";
import { activityHeadline, CompanyRecentList } from "../companyrecent";
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
 * The thread folded inside the 360: what happened lately, teased on one row
 * that opens it, with "Full history" beside for the History tab.
 *
 * CLOSED on arrival. It was open once, on the reasoning that the thread is
 * what the call above it was read FROM — true, but it cost the reading its
 * whole lower half before a reader had asked for anything, and the 360 exists
 * to be taken in at a glance. So the row teases instead: it names the section,
 * says how much is in it, and shows the newest exchange, which is the part a
 * reader wanted the thread for in the first place. A control that says only
 * "show" makes them open it to find out whether it was worth opening.
 *
 * The whole row is the control — it is a `Disclosure`, whose `<summary>` is
 * the toggle — so the label, the count and the teaser are all pressable rather
 * than a button sitting beside them. "Full history" goes in `action`, OUTSIDE
 * the summary, because a control inside a control both fails
 * `nested-interactive` and would fold the section it was meant to leave.
 *
 * It asks `sectionState` the same question the chronicle does, so a section
 * this reader may not see says so when opened rather than reading as an
 * account nobody has written to — and teases nothing, because there is nothing
 * it may honestly promise.
 */
export function ThreadFold({
  view,
  loading,
  onOpenHistory,
  onOpenRecord,
}: Readonly<{
  view?: Organization360;
  loading: boolean;
  onOpenHistory?: () => void;
  // The page's own router, the same door the spine above this fold takes. The
  // fold draws the account's messages in full — sender, subject, preview — so
  // without this it shows a reader the message and refuses to open it.
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const logged = view?.activities?.data ?? [];
  const state = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    logged.length,
    loading,
  );
  const shown = Math.min(logged.length, THREAD_LIMIT);
  // The newest exchange, in the words its own row will use once the fold
  // opens. Only when the section is READY: a teaser drawn from a withheld or
  // half-read section would state a fact the list under it then refuses.
  const newest = state === "ready" ? logged[0] : undefined;
  return (
    <PanelBody>
      <Disclosure
        className="co-thread-fold"
        summary={
          <>
            <span className="co-thread-name">
              {shown > 0
                ? t("co.360.threadCount", {
                    count: formatNumber(shown, locale),
                  })
                : t("co.360.thread")}
            </span>
            {newest && (
              <span className="co-thread-teaser">
                {activityHeadline(newest, t)}
                {" · "}
                {formatDateAbbrev(newest.occurred_at, locale, zone)}
              </span>
            )}
          </>
        }
        action={
          onOpenHistory && (
            <Button small variant="ghost" onClick={onOpenHistory}>
              {t("co.360.fullHistory")}
            </Button>
          )
        }
      >
        {state === "ready" ? (
          <CompanyRecentList
            activities={logged.slice(0, THREAD_LIMIT)}
            nameOf={recordNamesIn(view)}
            onOpenRecord={onOpenRecord}
          />
        ) : (
          <SurfaceState
            loadingLabel={t("co.recent.title")}
            state={state}
            emptyLabel={t("co.recent.empty")}
            emptyDetail={t("co.recent.emptyDetail")}
          >
            {null}
          </SurfaceState>
        )}
      </Disclosure>
    </PanelBody>
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
          <button type="button" className="link-button" onClick={onAllDeals}>
            {t("co.commercial.allDeals")}
          </button>
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
              loadingLabel={t("companyProjects.title")}
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
          <button
            type="button"
            className="link-button"
            onClick={() => onOpenTab("people")}
          >
            {count != null
              ? t("co.rail.all", { count: formatNumber(count, locale) })
              : t("co.rail.allUncounted")}
          </button>
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
          <SurfaceState
            state={state}
            emptyLabel={t("co.rail.people.empty")}
            loadingLabel={t("co.rail.people.title")}
          >
            {null}
          </SurfaceState>
        )}
      </PanelBody>
    </Panel>
  );
}
