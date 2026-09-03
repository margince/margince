// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight } from "lucide-react";
import { useRecordZone } from "../app/recordzone";
import { navigate, routeHash } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DealCard } from "../design-system/composed";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryGate } from "./common";
import { errorClassKey, isUnhealthy } from "./connector-status";
import { toBoardDeal, useOrgMarks } from "./deals";
import { EntityRef } from "./entityref";
import {
  type Deal,
  type MorningDigest,
  useMorningDigest,
  usePipelineValue,
} from "./home.queries";
import { isProjectPhase, PHASE_LABEL } from "./projects.form";

// Home's context rail: what happened, what the pipeline is worth, and what has
// gone quiet. Three panels, all of them READ — the work is in the main column
// beside them, and a rail that asks for a move is a second lead.
//
// Every panel is `Panel` rather than a card of its own, which is what makes the
// rail read as one column of the same shape: a header band at one height,
// full-bleed rows under it, and a footer only where a figure belongs to the
// whole panel.

type DigestProjects = NonNullable<MorningDigest["projects"]>;

// A rung of the project ladder in the reader's words. The digest carries the
// phase as open wire text, so a rung added upstream renders as its own word
// rather than failing the index.
function phaseWord(phase: string, t: (key: MessageKey) => string): string {
  return isProjectPhase(phase) ? t(PHASE_LABEL[phase]) : phase;
}

/** One labelled count inside the overnight panel. */
function DigestCount({
  label,
  value,
  onOpen,
}: Readonly<{ label: string; value: number; onOpen?: () => void }>) {
  const { locale } = useLocale();
  if (onOpen) {
    return (
      <PanelRow interactive>
        <button type="button" className="rail-count-go" onClick={onOpen}>
          <span className="rail-count-label">{label}</span>
          <span className="rail-count-value t-mono">
            {formatNumber(value, locale)}
          </span>
          <ArrowRight size={14} aria-hidden />
        </button>
      </PanelRow>
    );
  }
  return (
    <PanelRow>
      <span className="rail-count">
        <span className="rail-count-label">{label}</span>
        <span className="rail-count-value t-mono">
          {formatNumber(value, locale)}
        </span>
      </span>
    </PanelRow>
  );
}

// What moved on the projects overnight: every project named is a link to its
// page, because the section exists to send the reader there. A list that is
// empty renders nothing — the heading alone would claim news it has none of.
function DigestProjectsBlock({
  projects,
}: Readonly<{ projects: DigestProjects }>) {
  const t = useT();
  const { locale } = useLocale();
  const { phase_changes, new_commitments, gone_quiet } = projects;
  // The birth row of a project created overnight carries no from_phase; a move
  // between rungs is the news, so only those are listed.
  const moves = phase_changes.filter((change) => change.from_phase != null);
  if (
    moves.length === 0 &&
    new_commitments.length === 0 &&
    gone_quiet.length === 0
  ) {
    return null;
  }
  return (
    <PanelBody className="rail-projects">
      <span className="t-eyebrow">{t("home.digestProjects")}</span>
      {moves.length > 0 && (
        <ul
          className="rail-project-list"
          aria-label={t("home.digestPhaseChanges")}
        >
          {moves.map((change) => (
            <li key={`${change.project_id}-${change.occurred_at}`}>
              <EntityRef kind="project" id={change.project_id} />{" "}
              <span className="t-caption">
                {t("home.digestPhaseChange", {
                  from: phaseWord(change.from_phase ?? "", t),
                  to: phaseWord(change.to_phase, t),
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
      {new_commitments.length > 0 && (
        <ul
          className="rail-project-list"
          aria-label={t("home.digestNewCommitments")}
        >
          {new_commitments.map((item) => (
            <li key={item.project_id}>
              <EntityRef kind="project" id={item.project_id} />{" "}
              <span className="t-caption">
                {t("home.digestCommitmentCount", {
                  count: formatNumber(item.new_open_commitments, locale),
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
      {gone_quiet.length > 0 && (
        <ul
          className="rail-project-list"
          aria-label={t("home.digestGoneQuiet")}
        >
          {gone_quiet.map((item) => (
            <li key={item.project_id}>
              <EntityRef kind="project" id={item.project_id} />{" "}
              <span className="t-caption">
                {t("home.digestQuietDays", {
                  days: formatNumber(item.days_quiet, locale),
                })}
              </span>
            </li>
          ))}
        </ul>
      )}
    </PanelBody>
  );
}

/**
 * What the night shift did: capture counts, what it left for review, and the
 * one connector fact worth interrupting a morning for.
 *
 * Before the first nightly run there is no digest, and this panel is absent
 * rather than a row of zeros — a fabricated count is worse than a missing one,
 * because a reader cannot tell it apart from a real one.
 */
export function OvernightPanel() {
  const t = useT();
  const { locale } = useLocale();
  // The digest's own day is a fact about the installation's calendar, so it
  // reads the installation's zone rather than a constant.
  const recordZone = useRecordZone();
  const digestQuery = useMorningDigest();
  return (
    <QueryGate query={digestQuery}>
      {(digest) => {
        if (digest === null) {
          return null;
        }
        const { capture, review, connectors, projects } = digest;
        // A healthy connector is not news, and a permanent green row is noise.
        // Only an unhealthy one surfaces here, in Settings' own vocabulary so
        // the two surfaces never describe the same state differently.
        const unhealthy = connectors.filter(
          (c) => c.status != null && isUnhealthy(c.status),
        );
        return (
          <Panel
            title={t("home.panel.overnight")}
            sub={t("home.digestFor", {
              date: formatDate(digest.date, locale, recordZone),
            })}
            className="rail-panel"
          >
            <DigestCount
              label={t("home.digestSynced")}
              value={capture.messages_synced ?? 0}
            />
            <DigestCount
              label={t("home.digestPeople")}
              value={capture.people_created ?? 0}
            />
            <DigestCount
              label={t("home.digestOrgs")}
              value={capture.organizations_created ?? 0}
            />
            <DigestCount
              label={t("home.digestDedupe")}
              value={review.dedupe_open ?? 0}
              onOpen={() => navigate({ screen: "worklist" })}
            />
            <PanelBody>
              <p className="t-caption">
                {t("home.digestClassify", {
                  commitments: formatNumber(
                    review.classify?.commitments ?? 0,
                    locale,
                  ),
                  meetings: formatNumber(
                    review.classify?.meetings ?? 0,
                    locale,
                  ),
                  noise: formatNumber(review.classify?.noise ?? 0, locale),
                })}
              </p>
            </PanelBody>
            {projects && <DigestProjectsBlock projects={projects} />}
            {unhealthy.length > 0 && (
              <PanelBody>
                <Callout
                  tone="warn"
                  className="rail-connector-health"
                  actions={
                    <Button
                      small
                      onClick={() =>
                        navigate({ screen: "settings", id: "connections" })
                      }
                    >
                      {t("home.overnight.fixConnector")}
                    </Button>
                  }
                >
                  {t(errorClassKey(unhealthy[0].last_sync_error_class))}
                </Callout>
              </PanelBody>
            )}
          </Panel>
        );
      }}
    </QueryGate>
  );
}

/**
 * The open pipeline, one line per currency.
 *
 * A refusal is NOT an absence: an empty panel reads as "there is no pipeline",
 * which is a claim about the data made in place of a claim about authority. So a
 * settled error keeps the panel and says the figure is unavailable.
 */
export function PositionPanel() {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const query = usePipelineValue();

  // Still loading: no panel. A headline has no honest skeleton — a shape where a
  // number will be is a claim that a number is coming, and the work this rail
  // sits beside is what the reader is here for.
  if (query.isPending) {
    return null;
  }
  if (query.isError) {
    return (
      <Panel title={t("home.panel.position")} className="rail-panel">
        <PanelBody>
          <p className="t-caption">{t("home.pipelineUnavailable")}</p>
        </PanelBody>
      </Panel>
    );
  }
  const rows = query.data?.rows ?? [];
  const excluded = query.data?.excluded ?? 0;
  if (rows.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("home.panel.position")}
      className="rail-panel"
      footer={
        // A mask kept rows out of these sums, so the figures understate the
        // pipeline. Saying so is the difference between a partial answer and a
        // wrong one.
        excluded > 0 ? (
          <span className="t-caption">
            {t("home.pipelinePartial", {
              count: formatNumber(excluded, locale),
            })}
          </span>
        ) : undefined
      }
    >
      {rows.map((row) => (
        <PanelRow key={row.currency}>
          <span className="rail-money">
            <span className="rail-money-raw t-mono">
              {formatMoneyOrAbsent(row.rawMinor, row.currency, locale)}
            </span>
            <span className="t-caption">
              {t("home.pipelineWeighted", {
                amount: formatMoneyOrAbsent(
                  row.weightedMinor,
                  row.currency,
                  locale,
                ),
              })}
            </span>
            <span className="t-caption">
              {plural("home.pipelineCount", row.deals, {
                count: formatNumber(row.deals, locale),
              })}
            </span>
          </span>
        </PanelRow>
      ))}
    </Panel>
  );
}

/**
 * The open deals nobody has heard from.
 *
 * The company on each card is resolved through the SAME naming the pipeline
 * board uses (`useOrgMarks` + `toBoardDeal`), which is what gives a card its
 * four honest readings — named, withheld, unreadable, or genuinely no company.
 * Home used to pass `org: ""` unconditionally, so every quiet deal on this page
 * claimed to belong to no company at all.
 */
export function WatchPanel({
  deals,
  more,
  state,
}: Readonly<{
  deals: readonly Deal[];
  /** Whether Home's one page of deals ended short of the list. A quiet deal
   *  past that page is not on this panel, and `partial` is the state that says
   *  so — "nothing has gone quiet" would be a claim this read cannot make. */
  more: boolean;
  state: SectionState;
}>) {
  const t = useT();
  // No page of organizations to draw on here — this is a short list, and every
  // company it names is resolved by id and cached.
  const naming = useOrgMarks([...deals], [], true);
  if (state === "loading") {
    return null;
  }
  // "Nothing has gone quiet" is a CLAIM about the deals, so it may only be made
  // once they have been read. A failed read used to reach the same sentence,
  // which told a reader their pipeline was healthy on the strength of a request
  // that never answered.
  // Only a settled read may say anything about the deals, and WHICH thing it
  // says turns on whether the page ended the list: past it, an empty panel is
  // not "nothing has gone quiet", it is "nothing on the page we read".
  const settled = state === "ready";
  const resolved: SectionState = !settled
    ? state
    : more
      ? "partial"
      : deals.length === 0
        ? "empty"
        : state;
  return (
    <Panel title={t("home.panel.watch")} className="rail-panel">
      <PanelBody className={deals.length > 0 ? "rail-watch-list" : undefined}>
        <SurfaceState state={resolved} emptyLabel={t("home.watch.clear")}>
          {deals.map((deal) => (
            <DealCard
              key={deal.id}
              deal={toBoardDeal(deal, naming)}
              // A link, so no press handler: nothing here drags, and the
              // address is the whole behaviour.
              href={routeHash({ screen: "deals", id: deal.id })}
            />
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
