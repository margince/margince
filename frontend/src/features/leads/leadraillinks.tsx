// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ListPopover } from "../../design-system/listpopover";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { routeHash } from "../../lib/router";
// The card shapes this file draws (record-card, co-deal-card,
// co-project-card) are the account rail's own, defined in
// company360.css and companyrailprojects.css. Imported HERE rather than
// relied on from a sibling, for the same reason companyraildeals.tsx gives:
// this file renders wherever it is mounted.
import "../../design-system/recordcard.css";
import "../companies/company360.css";
import "../companies/companyrailprojects.css";
import { useEntityName } from "../../components/entityref";
import { problemMessageOf } from "../../lib/problem";
import { useProjectRecord } from "../projects/projectrecord";
import { PhaseBadge } from "../projects/projects";
import type { Project } from "../projects/projects.form";
import { searchProjectReferences } from "../records/recordreferences";
import type { LeadWriter } from "./leads";

// The lead's Deals tab: the deal it opened once qualified, and the project
// its details name.

type Lead = components["schemas"]["Lead"];

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

// The card, or the one sentence an unqualified lead gets instead.
function LeadDealBody({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { dealId, name, isEmpty } = useLeadDealLink(lead.qualified_deal_id);
  if (dealId && name) {
    return <DealRecordCard dealId={dealId} name={name} />;
  }
  return isEmpty ? <p>{t("lead.rail.deal.empty")}</p> : null;
}

// The verb the deal panel offers while there is still a deal to earn.
// `lead.promote`, the header's own key: qualifying is one action wherever it
// is reached from, so it carries one word everywhere.
function LeadQualifyButton({
  onQualify,
  reasonId,
}: Readonly<{ onQualify: () => void; reasonId?: string }>) {
  const t = useT();
  return (
    <div className="card-actions">
      <Button variant="ghost" reasonId={reasonId} onClick={onQualify}>
        {t("lead.promote")}
      </Button>
    </div>
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

// The card, or the sentence a lead with no project gets instead.
function LeadProjectBody({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { project, isEmpty } = useLeadProjectLink(lead.project_id);
  if (project) {
    return <ProjectRecordCard project={project} />;
  }
  return isEmpty ? <p>{t("lead.rail.project.empty")}</p> : null;
}

// Attaches `project_id` through the SAME writer the Details card's own
// project row saves through, reused rather than opening a second patch
// path, so one inline edit and this verb cannot invalidate different caches
// or send a different If-Match. Picking IS the act, so the list closes once
// the write lands and stays open over a refusal.
export function LeadProjectAttachButton({
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
  // alone would react to a save this picker never made (LeadScoreCard states
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
    <div className="card-actions">
      <ListPopover
        label={label}
        title={label}
        searchLabel={t("projectLinks.searchLabel")}
        reasonId={reasonId}
        open={open}
        onOpenChange={(next) => {
          if (next || !writer.patch.isPending) {
            setOpen(next);
          }
        }}
        search={searchProjectReferences}
        selected={lead.project_id ?? undefined}
        onPick={(option, done) => {
          if (option.id === lead.project_id) {
            done();
            return;
          }
          writer.save({ project_id: option.id });
        }}
        pending={writer.patch.isPending}
        error={failed ? problemMessageOf(writer.patch.error, t) : undefined}
      />
    </div>
  );
}

/**
 * LeadDealsTab: the deal this lead opened, or the way to open one, then the
 * project it names, with Attach/Change. A lead earns a deal by being
 * qualified, never by a second create here, so the deal panel's verb is
 * Qualify. It is gone once a deal is named or the lead is closed.
 */
export function LeadDealsTab({
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
      <Panel title={t("tab.deals")}>
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
