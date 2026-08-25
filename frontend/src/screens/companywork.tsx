// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account's work in flight: one line per open deal, one per live project,
// each written from that record's own facts.
//
// This replaced a written account brief. On an account carrying several
// engagements the brief blended them — correspondence about one project became
// a sentence about another, and a figure read out of the blend had nowhere to
// be checked. So a deal and a project are two stories here, and the structure
// says so: two groups under their own subheads, never interleaved, and the
// header above them counts and nothing else.
//
// Every line is a template over typed fields the server picked. Nothing on
// this card is model-written, which is what makes each line checkable against
// the record it links to.

import type { ReactNode } from "react";

import { navigate } from "../app/router";
import { useRecordZone } from "../app/recordzone";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  relativeDays,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import "./companywork.css";

type Organization360 = components["schemas"]["Organization360"];
type WorkDeal = components["schemas"]["Organization360Deal"];
type WorkProject = components["schemas"]["Organization360Project"];
type Attention = components["schemas"]["Organization360WorkAttention"];

/**
 * CompanyWorkCard is the overview's lead: what is moving on this account, and
 * for each piece of it, the one reason it wants a person today.
 *
 * The two halves are gated separately because the payload gates them
 * separately. A reader holding the deal grant but not the project one sees
 * the deals and is told the projects are withheld — never a count that folds
 * an unreadable half into a number, and never the fit panel in this card's
 * place, which would read as "there is no pipeline here, go read the fit".
 */
export function CompanyWorkCard({
  view,
  loading = false,
  onOpenRecord,
}: Readonly<{
  view?: Organization360;
  // The composite read's own pending flag — see sectionState's own doc.
  loading?: boolean;
  // Where a cited conversation opens. The page owns it, because the same
  // receipt is cited from several cards and two owners would mean two
  // receipts open over each other.
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const deals = view?.deals;
  const projects = view?.projects;
  const dealState = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const projectState = sectionState(
    view,
    "projects",
    Boolean(projects),
    liveWork(projects).length,
    loading,
  );
  return (
    <Panel
      title={t("co.work.title")}
      titleAction={<WorkCount view={view} />}
      footer={<SinceLastVisit view={view} />}
    >
      <WorkGroup
        label={t("co.work.deals")}
        state={dealState}
        emptyLabel={t("co.work.noDeals")}
      >
        {(deals?.data ?? []).map((deal) => (
          <DealLine key={deal.deal_id} deal={deal} />
        ))}
      </WorkGroup>
      <WorkGroup
        label={t("co.work.projects")}
        state={projectState}
        emptyLabel={t("co.work.noProjects")}
      >
        {liveWork(projects).map((project) => (
          <ProjectLine
            key={project.project_id}
            project={project}
            onOpenRecord={onOpenRecord}
          />
        ))}
      </WorkGroup>
      {view?.attention_withheld && (
        <PanelBody>
          <p className="t-caption co-work-incomplete">
            {t("co.work.statusesWithheld")}
          </p>
        </PanelBody>
      )}
    </Panel>
  );
}

/**
 * SinceLastVisit is what moved on this account while the reader was away, and
 * whether any of the account was withheld from them.
 *
 * It sat under the written account brief this card replaced, and it is not
 * about the brief — it is about the account, so it moves with the lead card
 * rather than going away with the prose. Withheld sections are named ONCE,
 * about the whole page, rather than as a refusal beside each line a reader
 * did not get.
 */
function SinceLastVisit({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  const since = newActivities(view);
  const first = firstVisit(view);
  const withheld = (view?.sections_omitted.length ?? 0) > 0;
  if (!first && since === 0 && !withheld) {
    return null;
  }
  return (
    <p className="co-work-foot">
      {/* Never both: on a first open the server counts every activity as new,
          and "14 new items" beside "you are opening this account for the
          first time" is the page contradicting itself. */}
      {first && <span className="t-caption">{t("co.since.first")}</span>}
      {!first && since > 0 && (
        <span className="t-caption">
          {t(
            since === 1 ? "co.read.newActivityOne" : "co.read.newActivityMany",
            { count: since },
          )}
        </span>
      )}
      {withheld && <span className="t-caption">{t("co.prep.withheld")}</span>}
    </p>
  );
}

// newActivities is how many landed since the reader's baseline.
//
// Zero and "not counted" are different answers and neither earns a line: a
// withheld section means nobody counted, and a counted zero means nothing
// happened — reporting either as news would be a claim the page cannot make.
function newActivities(view?: Organization360): number {
  if (!view || view.sections_omitted.includes("since_last_visit")) {
    return 0;
  }
  return view.since_last_visit?.new_activities ?? 0;
}

// firstVisit is true only when the account HAS a baseline section and it is
// empty. Read off an absent section it would turn data a reader's grants
// withheld into a claim about their own history.
function firstVisit(view?: Organization360): boolean {
  if (!view || view.sections_omitted.includes("since_last_visit")) {
    return false;
  }
  return Boolean(view.since_last_visit) && !view.since_last_visit?.baseline_at;
}

/**
 * hasWorkInFlight is whether this card has anything to say, which is what
 * decides between it and the growth-fit panel in the same slot.
 *
 * A WITHHELD half counts as work: a reader who may not see the projects has
 * not been told this account has none, and swapping in the fit panel would
 * tell them exactly that.
 */
export function hasWorkInFlight(view?: Organization360): boolean {
  if (!view) {
    return false;
  }
  const withheld =
    view.sections_omitted.includes("deals") ||
    view.sections_omitted.includes("projects");
  return (
    withheld ||
    (view.deals?.data.length ?? 0) > 0 ||
    liveWork(view.projects).length > 0
  );
}

// The projects this card is about: the ones still in motion. A closed project
// is history, and the same filter the project picker applies — one answer to
// "live", so the card and the picker cannot disagree.
function liveWork(projects?: readonly WorkProject[]): readonly WorkProject[] {
  return (projects ?? []).filter((project) => project.phase !== "closed");
}

// One group, in whichever state its own half of the payload is in. A withheld
// half says so where its rows would have been, so the other half still reads.
function WorkGroup({
  label,
  state,
  emptyLabel,
  children,
}: Readonly<{
  label: string;
  state: ReturnType<typeof sectionState>;
  emptyLabel: string;
  children: ReactNode;
}>) {
  if (state === "ready") {
    return (
      <>
        <PanelBody className="co-work-head">
          <Eyebrow as="h3">{label}</Eyebrow>
        </PanelBody>
        {children}
      </>
    );
  }
  return (
    <PanelBody>
      <SurfaceState label={label} state={state} emptyLabel={emptyLabel}>
        {null}
      </SurfaceState>
    </PanelBody>
  );
}

/**
 * WorkCount is the header's whole content: how much is in flight, as a count.
 *
 * Counts and facts only. The reasons live on the rows, where each one sits
 * beside the record it was read from — a summary sentence up here would be
 * the blended narrative this card replaced.
 *
 * It says nothing at all when either half is withheld. "0 in flight" to a
 * reader who can see the deals but not the projects is a false statement
 * about the account, not a partial one.
 */
function WorkCount({ view }: Readonly<{ view?: Organization360 }>) {
  const t = useT();
  if (!view || !view.deals || !view.projects) {
    return null;
  }
  const count = view.deals.data.length + liveWork(view.projects).length;
  // The project list is capped server-side; `projects_page.has_more` is how
  // the payload says the count is a floor rather than the number.
  const capped = view.deals.page.has_more || view.projects_page?.has_more;
  return (
    <span className="t-small">
      {capped
        ? t("co.work.countAtLeast", { count })
        : t("co.work.count", { count })}
    </span>
  );
}

function DealLine({ deal }: Readonly<{ deal: WorkDeal }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <PanelRow className="co-row">
      <button
        type="button"
        className="co-rowlink"
        onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </button>
      <span className="co-row-meta">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.amount?.amount_minor != null && (
          <span className="t-mono">
            {formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )}
          </span>
        )}
        {deal.expected_close_date && (
          <span>
            {t("co.work.closes", {
              date: formatDate(deal.expected_close_date, locale, zone),
            })}
          </span>
        )}
      </span>
      {/* One status clause per row, in the order that decides which one it is:
          an overdue task beats a stall, because a stall is the absence of a
          reason and an overdue task IS one. */}
      {deal.attention ? (
        <AttentionLine attention={deal.attention} />
      ) : (
        deal.stalled && <StatusLine>{t("co.work.stalled")}</StatusLine>
      )}
    </PanelRow>
  );
}

function ProjectLine({
  project,
  onOpenRecord,
}: Readonly<{
  project: WorkProject;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <PanelRow className="co-row">
      <button
        type="button"
        className="co-rowlink"
        onClick={() => navigate({ screen: "projects", id: project.project_id })}
      >
        {project.key ?? project.name}
      </button>
      <span className="co-row-meta">
        <span>{t(`project.phase.${project.phase}`)}</span>
        {project.target_end_date && (
          <span>
            {t("co.work.targetEnd", {
              date: formatDate(project.target_end_date, locale, zone),
            })}
          </span>
        )}
      </span>
      {project.attention ? (
        <AttentionLine
          attention={project.attention}
          onOpenRecord={onOpenRecord}
        />
      ) : (
        project.quiet && (
          <StatusLine>
            {t("co.work.quiet", {
              when: relativeDays(project.last_activity_at, t),
            })}
          </StatusLine>
        )
      )}
    </PanelRow>
  );
}

/**
 * AttentionLine is the row's one reason, as a whole sentence.
 *
 * Whole-sentence templates, never a phrase concatenated from halves: German
 * puts the verb where English does not, and a sentence assembled from pieces
 * is a sentence no translator can fix. That is also why there is a separate
 * key for the unnamed case rather than a name substituted with a blank.
 *
 * A commitment QUOTES the body rather than asserting over it. The body is
 * free text an extractor read out of a conversation — "we'll revisit after
 * legal" is a proposition, not a noun phrase — so a template reading "they
 * owe us {body}" produces nonsense, and quoting also shows the reader the
 * words that were extracted instead of a claim written on top of them.
 */
function AttentionLine({
  attention,
  onOpenRecord,
}: Readonly<{
  attention: Attention;
  onOpenRecord?: (entityType: string, entityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const due = attention.due_at ? formatDate(attention.due_at, locale, zone) : "";
  if (attention.kind === "overdue_task") {
    return (
      <StatusLine tone="warn">
        {attention.who
          ? t("co.work.overdueTask", {
              who: attention.who,
              title: attention.title,
              date: due,
            })
          : t("co.work.overdueTaskUnnamed", {
              title: attention.title,
              date: due,
            })}
      </StatusLine>
    );
  }
  const sentence = attention.who
    ? t("co.work.owesUs", { who: attention.who, body: attention.title })
    : t("co.work.owesUsUnnamed", { body: attention.title });
  const source = attention.source_activity_id;
  return (
    <StatusLine tone={attention.due_at ? "warn" : undefined}>
      {source && onOpenRecord ? (
        <button
          type="button"
          className="co-rowlink"
          onClick={() => onOpenRecord("activity", source)}
        >
          {sentence}
        </button>
      ) : (
        sentence
      )}
      {due && ` ${t("co.work.wasDue", { date: due })}`}
    </StatusLine>
  );
}

// The row's status clause, on its own line under the facts. A sentence in the
// meta row would read as one more chip beside the stage and the money; this
// is the row saying why it is here, so it gets its own line.
//
// `quiet` on the badge for the same reason: one per row, in a column of rows,
// and a stack of filled pills is decoration a reader learns to skip.
function StatusLine({
  tone,
  children,
}: Readonly<{ tone?: "warn"; children: ReactNode }>) {
  if (tone) {
    return (
      <span className="co-work-status">
        <Badge tone={tone} quiet>
          {children}
        </Badge>
      </span>
    );
  }
  return <span className="co-work-status t-caption">{children}</span>;
}
