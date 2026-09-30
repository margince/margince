// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead page's head, in the same three registers the contact page reads in
// (contactpage.tsx): an inline subtitle on the name's own line, a pills row
// under it, and a facts strip across the head. A rep who has read one record
// type finds the same shapes on the other, rather than a lead-only layout that
// happens to hold the same facts.

import { type ReactNode, useId, useState } from "react";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { ContactLink } from "../design-system/contactlink";
import { ErrorLine } from "../design-system/errorline";
import { IdentityLine } from "../design-system/identityline";
import { Fact, RecordFacts } from "../design-system/recordfacts";
import { Select } from "../design-system/select";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useMe } from "./common";
import {
  EntityRef,
  RosterPartialNote,
  useRoster,
  useRosterPartial,
} from "./entityref";
import { leadStatusLabel } from "./leadpresentation";
import { scoreReasonLabel } from "./leadreadings";
import type { LeadWriter } from "./leads";
import { sourceLabelFor } from "./leadsources";

type Lead = components["schemas"]["Lead"];

// The name's own line: what this lead does, and where, exactly the register
// the contact page's title-and-company subtitle reads in (contactpage.tsx's
// ContactSubtitle). A lead carries no company FK, company_name is free text,
// so unlike the contact's it is never a link.
export function LeadSubtitle({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  if (!lead.title && !lead.company_name) {
    return null;
  }
  return (
    <div className="record-sub record-sub-inline">
      {lead.title}
      {lead.title && lead.company_name ? " · " : ""}
      {lead.company_name}
    </div>
  );
}

// The ladder's colours for the pill: a terminal status reads in the closure's
// own tone (the same warning family the band and the readings tile already give
// a promoted or disqualified lead) rather than the live ladder's, so the pill
// agrees with the rest of the page about what a closed lead looks like.
//
// A live-state switch of its own rather than leadpresentation.tsx's private
// statusTone, because that file is frozen at its file-length waiver; exporting
// it and adding the terminal case there would grow it past the gate that
// holds it at that count.
function statusPillTone(
  status: Lead["status"],
): "accent" | "success" | "warning" | undefined {
  switch (status) {
    case "promoted":
    case "disqualified":
      return "warning";
    case "contacted":
      return "accent";
    case "engaged":
      return "success";
    default:
      return undefined;
  }
}

// The pills under the name: that this is a lead, and where it stands on the
// ladder. Nothing else rides here: the owner moved to the facts strip below,
// where a reader acts on it, and the address followed it.
export function LeadPulse({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  const t = useT();
  const label = leadStatusLabel(lead.status);
  // The same row shape the contact's marks take: pills side by side with a
  // space between, never stretched across the head.
  return (
    <IdentityLine separator="space">
      <Badge tone="accent">{t("lead.marker")}</Badge>
      <Badge tone={statusPillTone(lead.status)}>
        {label ? t(label) : lead.status}
      </Badge>
    </IdentityLine>
  );
}

// How one candidate reads in the assignment list. The viewer reads as "Me": a
// rep scanning this list looks for themselves, not for their own name among
// colleagues'. A user with no display name still has to be pickable, so the
// id stands in rather than rendering a blank row.
function candidateLabel(
  entry: Readonly<{ id: string; display_name?: string }>,
  meId: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (entry.id === meId) {
    return t("lead.assignToMe");
  }
  return entry.display_name ?? entry.id;
}

// The assignee list's four readings, as early returns rather than one chained
// conditional: a roster still arriving, a roster that failed, a workspace
// with nobody else in it, and the picker itself.
function AssigneePicker({
  roster,
  rosterPartial,
  candidates,
  meId,
  pending,
  onPick,
}: Readonly<{
  roster: ReturnType<typeof useRoster>;
  rosterPartial: boolean;
  candidates: readonly Readonly<{ id: string; display_name?: string }>[];
  meId: string | undefined;
  pending: boolean;
  onPick: (ownerId: string) => void;
}>) {
  const t = useT();
  if (roster.isPending) {
    return <span>{t("share.rosterLoading")}</span>;
  }
  if (roster.isError) {
    return (
      <div className="lead-line">
        <ErrorLine inline>{t("share.rosterErrorUsers")}</ErrorLine>
        <Button onClick={() => roster.refetch()}>{t("common.retry")}</Button>
      </div>
    );
  }
  // "Nobody else" is a claim about the WHOLE workspace, so only a roster read
  // to its end may make it. Over a walk that stopped early it would report a
  // lead as unassignable when the colleague to hand it to sits on a page
  // nothing here read.
  if (candidates.length === 0 && !rosterPartial) {
    return <span>{t("lead.assignNobodyElse")}</span>;
  }
  return (
    <>
      <Select
        aria-label={t("lead.assignTo")}
        placeholder={t("lead.assignChoose")}
        value=""
        disabled={pending}
        options={candidates.map((entry) => ({
          value: entry.id,
          label: candidateLabel(entry, meId, t),
        }))}
        onChange={onPick}
      />
      <RosterPartialNote partial={rosterPartial} />
    </>
  );
}

// Ownership: who holds the lead, and reassignment to any workspace user. The
// owner reads as a NAME: EntityRef resolves it off the shared `/users`
// roster and falls back to the id only while that load is in flight or when
// the viewer cannot see the roster, so a reader is never handed a bare uuid.
// Reassignment is a plain owner change (UC-E13-04): the server audits it and
// keeps whatever routing decision it overrides, so the only thing this
// control owes the reader is an honest list of who they can hand it to.
//
// No caption of its own: it is drawn inside the facts strip's Owner cell,
// whose own Eyebrow already says what this value is.
function LeadOwner({
  lead,
  meId,
  pending,
  onAssign,
  refusedReasonId,
}: Readonly<{
  lead: Lead;
  meId: string | undefined;
  pending: boolean;
  onAssign: (ownerId: string) => void;
  // The page's one sentence about why this lead takes no changes, while it
  // does not: assigning writes the owner, so it is refused by the same fact
  // as every other write here.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const pickerId = useId();
  const [picking, setPicking] = useState(false);
  const roster = useRoster("user", picking);
  const rosterPartial = useRosterPartial("user", picking);
  // Everyone but the current owner, with the VIEWER first: assigning to
  // yourself is the common case on a small team, and it is now an option in
  // this one control rather than a button of its own (ADR-0108 §5).
  const candidates = (roster.data ?? [])
    .filter((entry) => !("is_agent" in entry) || !entry.is_agent)
    .filter((entry) => entry.id !== lead.owner_id)
    .sort((a, b) => {
      if (a.id === meId) return -1;
      if (b.id === meId) return 1;
      return 0;
    });

  return (
    <div className="lead-stack">
      <div className="lead-line">
        {lead.owner_id ? (
          lead.owner_id === meId ? (
            <span>{t("lead.ownerYou")}</span>
          ) : (
            <EntityRef kind="user" id={lead.owner_id} />
          )
        ) : (
          <span>{t("lead.unassigned")}</span>
        )}
        {/* ONE control, not a button that assigns to you beside a button
            that reveals a picker nobody can see until they press it
            (ADR-0108 §5). The viewer is the first option because
            self-assignment is the common case on a small team. */}
        <Button
          variant="link"
          disabled={pending}
          reasonId={refusedReasonId}
          aria-expanded={picking}
          aria-controls={pickerId}
          onClick={() => setPicking(!picking)}
        >
          {t("lead.assign")}
        </Button>
      </div>

      <div id={pickerId}>
        {picking && (
          <AssigneePicker
            roster={roster}
            rosterPartial={rosterPartial}
            candidates={candidates}
            meId={meId}
            pending={pending}
            onPick={(value) => {
              onAssign(value);
              setPicking(false);
            }}
          />
        )}
      </div>
    </div>
  );
}

// The lead's owner, in the facts strip where a reader looks for it beside the
// record's other summary values.
//
// Its own component rather than inline in the header's props, because what
// makes it pressable is three separate questions and they belong beside each
// other rather than spread through a JSX attribute list.
function LeadOwnerControl({
  lead,
  writer,
  terminalReasonId,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  terminalReasonId: string;
}>) {
  const me = useMe();
  // Assignment asks a DIFFERENT question from editing, so it drops the PER-ROW
  // half of the editor's answer and keeps the rest. `writable` is false on a
  // lead nobody owns (the write arm being right), and gating on it would shut
  // the only door out of the unassigned queue.
  //
  // The other two axes still bind. useCanWrite is the object grant AND the
  // seat ceiling: a read seat, or one holding no `lead.update`, gets no
  // pressable control, because the server refuses them and a button that only
  // fails is worse than none. An archived lead is refused too: a terminal
  // record is nobody's to hand on.
  const mayAssign = useCanWrite("lead", "update");
  const refusedReasonId =
    lead.archived_at || !mayAssign ? terminalReasonId : undefined;
  return (
    <LeadOwner
      lead={lead}
      meId={me.data?.user?.id}
      refusedReasonId={refusedReasonId}
      pending={
        writer.patch.isPending ||
        writer.claim.isPending ||
        Boolean(refusedReasonId)
      }
      // A lead nobody owns is nobody's to change, so the PATCH this control
      // used to send for EVERY pick was refused for the one pick a rep makes
      // most: taking an unassigned lead. Picking yourself on an unowned lead
      // goes through the claim door, which is the write the server actually
      // admits; naming a colleague stays a patch, which the assignment gate
      // answers.
      onAssign={(ownerId) =>
        !lead.owner_id && ownerId === me.data?.user?.id
          ? writer.claim.mutate()
          : writer.save({ owner_id: ownerId })
      }
    />
  );
}

// The facts strip across the head: how to reach this lead, where it came
// from, and who holds it, the same register the contact page's own strip
// reads in (contactpage.tsx's ContactFacts), so a rep moving between record
// types keeps meeting the same row of cells. Owner keeps its Assign control
// live in the cell: the strip is a place a reader ACTS on this one value, not
// only reads it.
export function LeadFacts({
  lead,
  id,
  writer,
  terminalReasonId,
}: Readonly<{
  lead: Lead;
  id: string;
  writer: LeadWriter;
  terminalReasonId: string;
}>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  return (
    <RecordFacts>
      {lead.email && (
        <Fact label={t("history.field.email")}>
          <ContactLink
            kind="email"
            value={lead.email}
            record={{ entityType: "lead", entityId: id }}
            readOnly={Boolean(lead.archived_at)}
            className="link-button lead-email"
            textClassName="lead-email"
          />
        </Fact>
      )}
      {lead.company_name && (
        <Fact label={t("create.companyName")}>{lead.company_name}</Fact>
      )}
      <Fact label={t("lead.ownerLabel")}>
        <LeadOwnerControl
          lead={lead}
          writer={writer}
          terminalReasonId={terminalReasonId}
        />
      </Fact>
      <Fact label={t("lead.score")}>
        {formatNumber(lead.score, locale)} · {scoreReasonLabel(lead, t)}
      </Fact>
      <Fact label={t("list.created")}>
        {formatDateAbbrev(lead.created_at, locale, viewerZone())}
      </Fact>
      <Fact label={t("history.field.source")}>
        {sourceLabelFor(lead, undefined, t)}
      </Fact>
    </RecordFacts>
  );
}
