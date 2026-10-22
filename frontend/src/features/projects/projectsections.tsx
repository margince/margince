// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useRef } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge, Button } from "../../design-system/atoms";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../design-system/overviewcard";
import {
  type SectionState,
  SurfaceState,
  sectionState,
} from "../../design-system/surfacestate";
import { DEAL_STATUS_LABEL } from "../../format/dealstatus";
import { formatMoneyWhole, formatNumber } from "../../format/format";
import { formatDateAbbrev, formatDayMonth } from "../../format/format.dates";
import { daysPast } from "../../format/lateness";
import { formatMoneyOrWord } from "../../format/moneyword";
import { useLocale, usePlural, useT } from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { projectRoleLabel } from "../records/record360";
import { CONTRACT_STATUS_LABEL } from "./contractlifecycle";
import {
  AddProjectStakeholder,
  RemoveProjectStakeholder,
} from "./projectstakeholders";

// The project page's cards, each drawn from one composite read
// (GET /projects/{id}/360). Every card answers the same three questions in the
// same order (withheld, unavailable, empty) through `sectionState`. So a
// section a grant refused says so rather than drawing an empty list that reads
// as "there is none".

export type Project360 = components["schemas"]["Project360"];
type Project360Section = components["schemas"]["Project360Section"];
type Deal = components["schemas"]["Deal"];
type Contract = components["schemas"]["Contract"];
type Stakeholder = components["schemas"]["Project360Stakeholder"];
type Commitment = components["schemas"]["Project360Commitment"];

// Every section of the page the reader may lack a grant for. `documents`
// rides the project grant itself and is never in `sections_omitted`, so it is
// not in this union either.
type SectionKey = Project360Section | "documents";

export function stateOf(
  view: Project360 | undefined,
  section: SectionKey,
  present: boolean,
  count: number,
): SectionState {
  if (section === "documents") {
    // Documents ride the project grant: the page carries them whenever it
    // carries the project, so absent means the read failed, never withheld.
    if (!view) {
      return "unavailable";
    }
    return present ? (count === 0 ? "empty" : "ready") : "unavailable";
  }
  return sectionState(view, section, present, count);
}

/**
 * The card shape of the Overview. It shows the rows when the section is
 * `ready`, and its own empty sentence when there are none. A section the
 * reader may not read, or that failed, gets SurfaceState's sentence.
 */
export function OverviewSection({
  title,
  action,
  state,
  emptyLabel,
  foot,
  children,
}: Readonly<{
  title: string;
  action?: ReactNode;
  state: SectionState;
  emptyLabel: string;
  foot?: ReactNode;
  children: ReactNode;
}>) {
  return (
    <OverviewCard
      title={title}
      action={action}
      foot={foot}
      empty={state === "empty" ? emptyLabel : undefined}
      emptyAction={
        state === "empty" || state === "ready" ? undefined : (
          <SurfaceState
            state={state}
            emptyLabel={emptyLabel}
            loadingLabel={title}
          >
            {null}
          </SurfaceState>
        )
      }
    >
      {state === "ready" && <OverviewList>{children}</OverviewList>}
    </OverviewCard>
  );
}

/** The deals rolled up to the project, newest first, every status. */
export function ProjectDealsCard({
  view,
  actions,
}: Readonly<{ view: Project360 | undefined; actions?: ReactNode }>) {
  const t = useT();
  const deals = view?.deals?.data ?? [];
  const state = stateOf(view, "deals", Boolean(view?.deals), deals.length);
  return (
    <OverviewSection
      title={t("project.deals.title")}
      action={state === "ready" || state === "empty" ? actions : undefined}
      state={state}
      emptyLabel={t("project.deals.empty")}
      foot={view?.deals?.page.has_more && t("project.deals.more")}
    >
      {deals.map((deal) => (
        <ProjectDealRow key={deal.id} deal={deal} />
      ))}
    </OverviewSection>
  );
}

// The stage an open deal stands in, by name, from the pipelines the reader
// may read; absent when the stage cannot be read.
function useStageName(stageId: string | null): string | undefined {
  const pipelines = usePipelines().data ?? [];
  return pipelines
    .flatMap((pipeline) => pipeline.stages ?? [])
    .find((stage) => stage.id === stageId)?.name;
}

function ProjectDealRow({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const stage = useStageName(deal.stage_id);
  const closed = deal.closed_at
    ? formatDayMonth(deal.closed_at, locale, recordZone)
    : undefined;
  let sub = t(DEAL_STATUS_LABEL[deal.status]);
  if (deal.status === "open" && stage) {
    sub = t("project.deals.openAt", { stage });
  } else if (deal.status !== "open" && closed) {
    sub = t(
      deal.status === "won" ? "project.deals.wonOn" : "project.deals.lostOn",
      { date: closed },
    );
  }
  return (
    <OverviewRow
      title={
        <button
          type="button"
          className="project-rowlink"
          onClick={() => navigate({ screen: "deals", id: deal.id })}
        >
          {deal.name}
        </button>
      }
      sub={sub}
      right={formatMoneyOrWord(
        deal.amount_minor,
        deal.currency,
        locale,
        "",
        formatMoneyWhole,
      )}
      figure
    />
  );
}

/**
 * The contacts seated on the project, each with the seat they hold, and the
 * verbs that put them there.
 *
 * The verbs ride the card's action, which the card draws only on a section
 * that is ready or empty. That is deliberate rather than incidental: a reader
 * whose grant withheld the seats is told so, and offering them an Add button
 * over a list they were not allowed to read would invite a write against
 * records they cannot see.
 */
export function StakeholdersCard({
  view,
  projectId,
  readOnly,
}: Readonly<{
  view: Project360 | undefined;
  projectId: string;
  // An archived project is read-only: the endpoints refuse a write against a
  // non-live row, so a verb here could only ever fail.
  readOnly?: boolean;
}>) {
  const t = useT();
  const seats = view?.stakeholders?.data ?? [];
  const state = stateOf(
    view,
    "stakeholders",
    Boolean(view?.stakeholders),
    seats.length,
  );
  const writable = !readOnly && (state === "ready" || state === "empty");
  // Where focus goes after a removal. The row holding the Remove button is gone
  // by then, and focus restored to a node that has unmounted leaves a keyboard
  // reader on document.body — so it lands on the panel's own Add control, which
  // survives every removal and is rendered exactly when a Remove button is.
  const addVerb = useRef<HTMLDivElement>(null);
  return (
    <OverviewSection
      title={t("project.stakeholders.title")}
      state={state}
      emptyLabel={t("project.stakeholders.empty")}
      action={
        writable ? (
          <div ref={addVerb}>
            <AddProjectStakeholder projectId={projectId} />
          </div>
        ) : undefined
      }
    >
      {seats.map((seat) => (
        <StakeholderRow
          key={seat.relationship_id}
          seat={seat}
          projectId={projectId}
          writable={writable}
          returnFocusTo={() =>
            addVerb.current?.querySelector<HTMLElement>("button") ?? null
          }
        />
      ))}
    </OverviewSection>
  );
}

function StakeholderRow({
  seat,
  projectId,
  writable,
  returnFocusTo,
}: Readonly<{
  seat: Stakeholder;
  projectId: string;
  writable: boolean;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  return (
    <OverviewRow
      title={
        <EntityRef
          kind="contact"
          id={seat.contact_id}
          name={seat.contact_name}
        />
      }
      sub={seat.role && projectRoleLabel(seat.role, t)}
      right={
        writable && (
          <RemoveProjectStakeholder
            projectId={projectId}
            contactId={seat.contact_id}
            contactName={seat.contact_name}
            returnFocusTo={returnFocusTo}
          />
        )
      }
    />
  );
}

/**
 * The agreements filed under the project. The files the project holds are a
 * step away, behind the card's Documents link.
 */
export function ProjectContractsCard({
  view,
  onOpenDocuments,
}: Readonly<{ view: Project360 | undefined; onOpenDocuments?: () => void }>) {
  const t = useT();
  const contracts = view?.contracts?.data ?? [];
  const state = stateOf(
    view,
    "contracts",
    Boolean(view?.contracts),
    contracts.length,
  );
  return (
    <OverviewSection
      title={t("project.contracts.title")}
      action={
        onOpenDocuments && (
          <Button variant="link" onClick={onOpenDocuments}>
            {t("project.documents.title")}
          </Button>
        )
      }
      state={state}
      emptyLabel={t("project.contracts.empty")}
    >
      {contracts.map((contract) => (
        <ContractRow key={contract.id} contract={contract} />
      ))}
    </OverviewSection>
  );
}

function ContractRow({ contract }: Readonly<{ contract: Contract }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const signed = contract.signed_on
    ? formatDayMonth(contract.signed_on, locale, zone)
    : undefined;
  const ends = contract.ends_on
    ? formatDateAbbrev(contract.ends_on, locale, zone)
    : undefined;
  let sub: string | undefined;
  if (signed && ends) {
    sub = t("project.contracts.signedRuns", { signed, ends });
  } else if (signed) {
    sub = t("project.contracts.signed", { date: signed });
  } else if (ends) {
    sub = t("project.contracts.runsTo", { date: ends });
  }
  return (
    <OverviewRow
      title={contract.title || contract.contract_number}
      sub={sub}
      right={
        <Badge tone={contract.status === "active" ? "success" : undefined}>
          {t(CONTRACT_STATUS_LABEL[contract.status])}
        </Badge>
      }
    />
  );
}

/** The open tasks filed under the project, soonest due first. */
export function CommitmentsCard({
  view,
  meId,
  tick,
  onOpenTask,
  onOpenAll,
}: Readonly<{
  view: Project360 | undefined;
  // The signed-in user, so their own tasks read "You".
  meId?: string;
  // The complete box of one task, for a reader who may write tasks.
  tick?: (activityId: string) => ReactNode;
  // Opens one commitment's task detail. Owned by the screen rather than by
  // this card, because the modal is one per page.
  onOpenTask?: (activityId: string) => void;
  // Opens every task of the project, done ones included.
  onOpenAll?: () => void;
}>) {
  const t = useT();
  const commitments = view?.commitments?.data ?? [];
  const state = stateOf(
    view,
    "commitments",
    Boolean(view?.commitments),
    commitments.length,
  );
  return (
    <OverviewSection
      title={t("record.commitments")}
      action={
        onOpenAll && (
          <Button variant="link" onClick={onOpenAll}>
            {t("project.commitments.all")}
          </Button>
        )
      }
      state={state}
      emptyLabel={t("project.commitments.empty")}
    >
      {view &&
        commitments.map((commitment) => (
          <CommitmentRow
            key={commitment.activity_id}
            commitment={commitment}
            asOf={view.as_of}
            mine={
              commitment.assignee_id !== null && commitment.assignee_id === meId
            }
            tick={tick}
            onOpenTask={onOpenTask}
          />
        ))}
    </OverviewSection>
  );
}

function CommitmentRow({
  commitment,
  asOf,
  mine,
  tick,
  onOpenTask,
}: Readonly<{
  commitment: Commitment;
  asOf: string;
  mine: boolean;
  tick?: (activityId: string) => ReactNode;
  onOpenTask?: (activityId: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // The record's clock, exactly as the tasks screen reads it. A commitment is a
  // deadline the team agreed, so the day it names cannot depend on where the
  // reader is sitting.
  const recordZone = useRecordZone();
  // The server's `overdue` is the verdict; the days only word it.
  const late = commitment.due_at
    ? daysPast(Date.parse(commitment.due_at), Date.parse(asOf)).days
    : 0;
  const who = mine ? t("project.standing.you") : commitment.assignee_name;
  const sub = [
    who,
    commitment.due_at &&
      t("project.standing.event.due", {
        date: formatDayMonth(commitment.due_at, locale, recordZone),
      }),
  ].filter(Boolean);
  return (
    <OverviewRow
      lead={tick?.(commitment.activity_id)}
      title={
        onOpenTask ? (
          <button
            type="button"
            className="project-rowlink"
            onClick={() => onOpenTask(commitment.activity_id)}
          >
            {commitment.subject}
          </button>
        ) : (
          commitment.subject
        )
      }
      sub={sub.length > 0 ? sub.join(" · ") : undefined}
      right={
        commitment.overdue && (
          <Badge tone="danger">
            {late > 0
              ? plural("project.standing.daysLate", late, {
                  count: formatNumber(late, locale),
                })
              : t("project.commitments.overdue")}
          </Badge>
        )
      }
    />
  );
}
