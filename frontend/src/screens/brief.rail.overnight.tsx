// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight } from "lucide-react";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { formatDate, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type MorningDigest, useMorningDigest } from "./brief.queries";
import { QueryGate } from "./common";
import { errorClassKey, isUnhealthy } from "./connector-status";
import { EntityRef } from "./entityref";
import { isProjectPhase, PHASE_LABEL } from "./projects.form";

// What the night shift did, as one panel of Brief's context rail. Its own file
// because the rail was at its line ceiling and this is the half of it with a
// shape of its own: counts with doors, a projects block, and the one connector
// fact worth interrupting a morning for. `screens/brief.rail.tsx` re-exports
// the panel, so the rail is still assembled from one import.

type DigestProjects = NonNullable<MorningDigest["projects"]>;

/**
 * Whether the overnight read has ANSWERED with no digest behind it.
 *
 * `null` is the answer /v1/digest gives before the first nightly run and on an
 * installation that does not implement it; `undefined` is a read still in
 * flight, which is not an absence. The panel and the rail's quiet line both
 * turn on this, which is why it is a predicate rather than a comparison
 * written twice — and typed `digest is null` so the panel's own body narrows
 * off it instead of testing the same thing again.
 */
export function overnightIsEmpty(
  digest: MorningDigest | null | undefined,
): digest is null {
  return digest === null;
}

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
      <span className="t-eyebrow">{t("brief.digestProjects")}</span>
      {moves.length > 0 && (
        <ul
          className="rail-project-list"
          aria-label={t("brief.digestPhaseChanges")}
        >
          {moves.map((change) => (
            <li key={`${change.project_id}-${change.occurred_at}`}>
              <EntityRef kind="project" id={change.project_id} />{" "}
              <span className="t-caption">
                {t("brief.digestPhaseChange", {
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
          aria-label={t("brief.digestNewCommitments")}
        >
          {new_commitments.map((item) => (
            <li key={item.project_id}>
              <EntityRef kind="project" id={item.project_id} />{" "}
              <span className="t-caption">
                {t("brief.digestCommitmentCount", {
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
          aria-label={t("brief.digestGoneQuiet")}
        >
          {gone_quiet.map((item) => (
            <li key={item.project_id}>
              <EntityRef kind="project" id={item.project_id} />{" "}
              <span className="t-caption">
                {t("brief.digestQuietDays", {
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
 * because a reader cannot tell it apart from a real one. The rail's quiet panel
 * carries the line that says the digest is not there.
 */
export function OvernightPanel() {
  const t = useT();
  const digestQuery = useMorningDigest();
  return (
    <QueryGate query={digestQuery} pendingLabel={t("brief.panel.overnight")}>
      {(digest) =>
        overnightIsEmpty(digest) ? null : <DigestBody digest={digest} />
      }
    </QueryGate>
  );
}

/** The digest, once there is one. */
function DigestBody({ digest }: Readonly<{ digest: MorningDigest }>) {
  const t = useT();
  const { locale } = useLocale();
  // The digest's own day is a fact about the installation's calendar, so it
  // reads the installation's zone rather than a constant.
  const recordZone = useRecordZone();
  const { capture, review, connectors, projects } = digest;
  // A healthy connector is not news, and a permanent green row is noise. Only
  // an unhealthy one surfaces here, in Settings' own vocabulary so the two
  // surfaces never describe the same state differently.
  const unhealthy = connectors.filter(
    (c) => c.status != null && isUnhealthy(c.status),
  );
  return (
    <Panel
      title={t("brief.panel.overnight")}
      sub={t("brief.digestFor", {
        date: formatDate(digest.date, locale, recordZone),
      })}
      className="rail-panel"
    >
      <DigestCount
        label={t("brief.digestSynced")}
        value={capture.messages_synced ?? 0}
      />
      {/* The two counts that name a set of RECORDS, so they open it. The door
          sorts newest-first rather than bounding by date: `created_at` is a
          declared sort on both lists and neither endpoint has a "created on
          this day" filter, so overnight's rows land at the top of a longer list
          rather than being the whole of it. That is a superset, which is the
          honest direction to be wrong in — the reader sees the ones this figure
          counted and more besides, not fewer. The messages-synced count above
          has no list surface at all and keeps no door. */}
      <DigestCount
        label={t("brief.digestContacts")}
        value={capture.people_created ?? 0}
        onOpen={() =>
          navigate({ screen: "contacts" }, new Map([["sort", "-created_at"]]))
        }
      />
      <DigestCount
        label={t("brief.digestOrgs")}
        value={capture.organizations_created ?? 0}
        onOpen={() =>
          navigate({ screen: "companies" }, new Map([["sort", "-created_at"]]))
        }
      />
      <DigestCount
        label={t("brief.digestDedupe")}
        value={review.dedupe_open ?? 0}
        onOpen={() => navigate({ screen: "worklist" })}
      />
      <PanelBody>
        <p className="t-caption">
          {t("brief.digestClassify", {
            commitments: formatNumber(
              review.classify?.commitments ?? 0,
              locale,
            ),
            meetings: formatNumber(review.classify?.meetings ?? 0, locale),
            noise: formatNumber(review.classify?.noise ?? 0, locale),
          })}
        </p>
      </PanelBody>
      {projects && <DigestProjectsBlock projects={projects} />}
      {unhealthy.length > 0 && (
        <PanelBody>
          {/* EVERY broken connector, not the first one: naming `unhealthy[0]`
              alone told a reader with two dead mailboxes about one of them, and
              the digest is where they find out at all. `event`, because a sync
              failed overnight rather than under the reader's hand. */}
          <Callout
            tone="warn"
            kind="event"
            title={t("brief.overnight.connectorsUnhealthy")}
            actions={
              <Button
                small
                onClick={() =>
                  navigate({ screen: "settings", id: "connections" })
                }
              >
                {t("brief.overnight.fixConnector")}
              </Button>
            }
          >
            <ul>
              {unhealthy.map((connector) => (
                <li key={connector.provider}>
                  {t(errorClassKey(connector.last_sync_error_class))}
                </li>
              ))}
            </ul>
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
}
