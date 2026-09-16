// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useState } from "react";
import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import { Button, Disclosure } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
// The row and section shapes this file draws (record-card, co-deal-card,
// co-project-card, co-sect) are the account rail's own, defined in
// company360.css and companyrailprojects.css. Imported HERE rather than
// relied on from a sibling, for the same reason companyraildeals.tsx gives:
// this file renders wherever it is mounted.
import "../design-system/recordcard.css";
import "./company360.css";
import "./companyrailprojects.css";
import { problemMessageOf } from "./common";
import { SectionSummary } from "./companyrailshared";
import { useEntityName } from "./entityref";
import type { LeadWriter } from "./leads";
import { useProjectRecord } from "./projectrecord";
import { PhaseBadge } from "./projects";
import type { Project } from "./projects.form";
import { searchProjectReferences } from "./recordreferences";

// The lead's own two doors out: the deal it opened once qualified, and the
// project its Details name. Split out of leads.tsx so that file's own length
// holds: the rail slices, the tab they open onto, and the rail's own mount
// all live here, and leads.tsx only wires them to the record it reads.

type Lead = components["schemas"]["Lead"];

/**
 * LeadRail: the lead's own words, plus what it links to.
 *
 * What a rep CONSULTS while doing the work in the column beside it. Two
 * things have left this column for the same reason: the score, which is a
 * reading with an edit behind it and belongs beside the inputs that feed it,
 * and the owner, which is a thing a reader ACTS on and belongs in the header.
 *
 * `details` is LeadIdentityFields, kept private to leads.tsx: this file only
 * reserves it a slot, so the record's editable fields stay beside the patch
 * that saves them.
 */
export function LeadRail({
  lead,
  writer,
  onQualify,
  reasonId,
  details,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  // Opens QualifyDialog, the SAME dialog the header's own Qualify verb
  // opens: a lead earns a deal by being qualified, never by a second door
  // here.
  onQualify: () => void;
  // The one sentence this page prints about why the lead takes no changes,
  // when it is closed or not this caller's to change; undefined otherwise.
  reasonId?: string;
  details: ReactNode;
}>) {
  return (
    // The account rail's own shape: the record's fields lead the column, and
    // what it links to stands under them as ONE pane of named slices rather
    // than as two cards a reader has to assemble. `.co-rail` is what gives a
    // slice its eyebrow and its hairline, so the two records read here the
    // way the same two read on an account.
    <div className="co-rail">
      {details}
      <Panel>
        <LeadDealSection
          lead={lead}
          onQualify={onQualify}
          reasonId={reasonId}
        />
        <LeadProjectSection lead={lead} writer={writer} reasonId={reasonId} />
      </Panel>
    </div>
  );
}

// The deal named by `lead.qualified_deal_id`, read the way every reference to
// a record this page does not own is read: by name only (entityref.tsx), off
// the same cache the Details row's own project reference shares. `isEmpty` is
// true both when the lead carries no deal yet and when the deal it named
// cannot be read (gone, or hidden by row-scope), so a reader gets one settled
// sentence either way, never a raw id or a permission complaint.
function useLeadDealLink(dealId: string | null | undefined) {
  const { name, reading } = useEntityName("deal", dealId);
  const isEmpty = !dealId || reading === "unnamed";
  return { dealId, name, isEmpty };
}

// The same card face the account rail's own deal row wears
// (company360.css's `.co-deal-card`, over `recordcard.css`'s `.record-card`):
// the whole card is the link, and a deal card carries no other control to
// collide with it.
function DealRecordCard({
  dealId,
  name,
}: Readonly<{ dealId: string; name: string }>) {
  return (
    <a
      className="record-card co-deal-card"
      href={routeHash({ screen: "deals", id: dealId })}
      aria-label={name}
    >
      <span className="record-card-name">{name}</span>
    </a>
  );
}

// The card, or the one sentence an unqualified lead gets instead. Shared by
// the rail slice and the tab's own panel, which read it identically: the
// deal is never edited from either surface, so there is nothing for the two
// to disagree about.
function LeadDealBody({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { dealId, name, isEmpty } = useLeadDealLink(lead.qualified_deal_id);
  if (dealId && name) {
    return <DealRecordCard dealId={dealId} name={name} />;
  }
  return isEmpty ? (
    <p className="t-caption">{t("lead.rail.deal.empty")}</p>
  ) : null;
}

// The verb this lead's deal slice offers, when there is still a deal to
// earn, shared by the rail slice and the tab's own panel, which draw it
// identically. `lead.promote`, the header's own key: qualifying is one
// action wherever it is reached from, so it carries one word everywhere.
function LeadQualifyButton({
  onQualify,
  reasonId,
}: Readonly<{ onQualify: () => void; reasonId?: string }>) {
  const t = useT();
  return (
    <div className="card-actions">
      <Button small variant="ghost" reasonId={reasonId} onClick={onQualify}>
        {t("lead.promote")}
      </Button>
    </div>
  );
}

/**
 * LeadDealSection is the deal this lead became, when it became one. A lead
 * earns a deal by being qualified, never by a second create here, so the
 * verb this slice offers IS Qualify, the same door the header's own Qualify
 * action opens, and drawn on the same terms the header draws it on: gone
 * once `qualified_deal_id` is set (the card above names the deal) or once
 * the lead is closed, since a closed lead is not a qualifiable one either:
 * the reason band is for a verb that STAYS, not for one that no longer
 * applies.
 */
export function LeadDealSection({
  lead,
  onQualify,
  reasonId,
}: Readonly<{
  lead: Lead;
  onQualify: () => void;
  reasonId?: string;
}>) {
  const t = useT();
  return (
    <Disclosure
      className="co-sect"
      open
      summary={<SectionSummary title={t("lead.rail.deal.title")} />}
    >
      <PanelBody>
        <LeadDealBody lead={lead} />
        {!lead.qualified_deal_id && !lead.archived_at && (
          <LeadQualifyButton onQualify={onQualify} reasonId={reasonId} />
        )}
      </PanelBody>
    </Disclosure>
  );
}

// The project named by `lead.project_id`, read the way the Details row
// already resolves it (`useProjectRecord`, the richer read the composer's
// subject tag shares), since the phase badge needs more than a bare name.
// `isEmpty` follows the same rule as the deal's: no id, or an id that no
// longer resolves, both read as nothing to show.
function useLeadProjectLink(projectId: string | null | undefined) {
  const { project, settled } = useProjectRecord(projectId ?? undefined);
  const isEmpty = !projectId || (settled && !project);
  return { project, isEmpty };
}

// The same card face the deal row wears, with the phase badge sharing the
// name's own row the way the account rail's project row draws it
// (companyrailprojects.css's `.co-project-card`/`.co-project-phase`).
function ProjectRecordCard({ project }: Readonly<{ project: Project }>) {
  return (
    <a
      className="record-card co-project-card"
      href={routeHash({ screen: "projects", id: project.id })}
      aria-label={project.name}
    >
      <span className="record-card-name">{project.name}</span>
      <span className="co-project-phase">
        <PhaseBadge phase={project.phase} />
      </span>
    </a>
  );
}

// The card, or the sentence a lead with no project gets instead, shared by
// the rail slice and the tab's own panel, the same split `LeadDealBody`
// draws.
function LeadProjectBody({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { project, isEmpty } = useLeadProjectLink(lead.project_id);
  if (project) {
    return <ProjectRecordCard project={project} />;
  }
  return isEmpty ? (
    <p className="t-caption">{t("lead.rail.project.empty")}</p>
  ) : null;
}

// Attaches `project_id` through the SAME writer the Details card's own
// project row saves through, reused rather than opening a second patch
// path, so one inline edit and this verb cannot invalidate different caches
// or send a different If-Match. Picking IS the act, the shape the account's
// own project attach draws (design-system/projectlinks.tsx's AttachDialog):
// the picker's confirm never fires, kept only so a refused write has
// ConfirmModal's pending and error slots to appear in.
function LeadProjectAttachButton({
  lead,
  writer,
  reasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; reasonId?: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const label = lead.project_id
    ? t("lead.rail.project.change")
    : t("lead.rail.project.attach");
  // THIS write's own outcome, not any write on the lead: the mutation is
  // shared with the Details card and the ladder, so `isSuccess`/`isError`
  // alone would react to a save this dialog never made (LeadScoreCard states
  // the same rule for its own override save).
  const thisWrite =
    writer.patch.variables?.body !== undefined &&
    "project_id" in writer.patch.variables.body;
  const saved = writer.patch.isSuccess && thisWrite;
  const failed = writer.patch.isError && thisWrite;
  useEffect(() => {
    if (saved) {
      setOpen(false);
    }
  }, [saved]);
  return (
    <>
      <div className="card-actions">
        <Button
          small
          variant="ghost"
          reasonId={reasonId}
          onClick={() => setOpen(true)}
        >
          {label}
        </Button>
      </div>
      <ConfirmModal
        open={open}
        onClose={() => {
          if (!writer.patch.isPending) {
            setOpen(false);
          }
        }}
        title={label}
        confirmLabel={label}
        confirmDisabled
        onConfirm={() => undefined}
        pending={writer.patch.isPending}
        error={failed ? problemMessageOf(writer.patch.error, t) : null}
      >
        <RecordPicker
          label={t("projectLinks.searchLabel")}
          searchTargets={searchProjectReferences}
          disabled={writer.patch.isPending}
          onPick={(candidate: RecordPickerCandidate) =>
            writer.save({ project_id: candidate.id })
          }
        />
      </ConfirmModal>
    </>
  );
}

/**
 * LeadProjectSection is the project this lead names, the one field of
 * `project_id` this rail draws as more than a select row. It carries the
 * verb that writes that field: Attach while the lead names none, Change once
 * it does, the same picker either way, and the Details row keeps its own
 * editor beside it rather than losing one.
 */
export function LeadProjectSection({
  lead,
  writer,
  reasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; reasonId?: string }>) {
  const t = useT();
  return (
    <Disclosure
      className="co-sect"
      open
      summary={<SectionSummary title={t("lead.rail.project.title")} />}
    >
      <PanelBody>
        <LeadProjectBody lead={lead} />
        <LeadProjectAttachButton
          lead={lead}
          writer={writer}
          reasonId={reasonId}
        />
      </PanelBody>
    </Disclosure>
  );
}

/**
 * LeadDealsProjectsTab: the lead's Deals & projects tab, the same label the
 * account's own tab carries. Both panels carry the same verb the rail's own
 * slices do, Qualify and Attach/Change, so a rep reaching either surface
 * finds the same door.
 */
export function LeadDealsProjectsTab({
  lead,
  writer,
  onQualify,
  reasonId,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  onQualify: () => void;
  reasonId?: string;
}>) {
  const t = useT();
  return (
    <div className="record-stack">
      <Panel title={t("lead.rail.deal.title")}>
        <PanelBody>
          <LeadDealBody lead={lead} />
          {!lead.qualified_deal_id && !lead.archived_at && (
            <LeadQualifyButton onQualify={onQualify} reasonId={reasonId} />
          )}
        </PanelBody>
      </Panel>
      <Panel title={t("lead.rail.project.title")}>
        <PanelBody>
          <LeadProjectBody lead={lead} />
          <LeadProjectAttachButton
            lead={lead}
            writer={writer}
            reasonId={reasonId}
          />
        </PanelBody>
      </Panel>
    </div>
  );
}
