// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead page's head, in the registers the contact page reads in: one fact
// per line under the name, then a pills row. Who holds the lead, and how it
// reached its status, are the details' Ownership rows.

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { mayOwnRecords } from "../../components/assigneepicker";
import {
  EntityRef,
  RosterFailedLine,
  useRoster,
  useRosterPartial,
  useRosterPartialHint,
} from "../../components/entityref";
import { fetchGrants } from "../../components/share.parts";
import { Badge } from "../../design-system/atoms";
import { EditableValue } from "../../design-system/editablevalue";
import {
  type FieldEditControl,
  FieldEditDialog,
} from "../../design-system/fieldeditdialog";
import { IdentityLine } from "../../design-system/identityline";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { LeadCompany } from "./leadcompany";
import { leadStatusLabel } from "./leadpresentation";
import type { LeadWriter } from "./leads";

type Lead = components["schemas"]["Lead"];

// Under the name, one fact per line: what this lead does, and where. The
// company is a link once a human linked it, and the typed name before.
export function LeadSubtitle({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  return (
    <>
      {lead.title && <span>{lead.title}</span>}
      {(lead.company_id || lead.company_name?.trim()) && (
        <span>
          <LeadCompany lead={lead} />
        </span>
      )}
    </>
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

// Who else may open the lead, read off its grants: "Shared" once one exists,
// "Not shared" before. A reader the grants are not shown to gets no pill,
// rather than a guess. Pressing it opens the share screen.
function LeadSharePill({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  const t = useT();
  const grants = useQuery({
    queryKey: ["record-grants", "lead", lead.id],
    queryFn: () => fetchGrants("lead", lead.id),
  });
  if (!grants.isSuccess) {
    return null;
  }
  const label =
    (grants.data?.length ?? 0) > 0
      ? t("visibility.workspace")
      : t("record.details.notShared");
  // A lead nobody may change cannot be shared further, the same refusal the
  // menu's Share verb states. So its pill is a fact and not a door.
  if (lead.archived_at || lead.writable === false) {
    return <Badge>{label}</Badge>;
  }
  return (
    <button
      type="button"
      className="lead-share-pill"
      onClick={() => navigate({ screen: "share", id: "lead", id2: lead.id })}
    >
      <Badge>{label}</Badge>
      <ChevronDown aria-hidden="true" />
    </button>
  );
}

// The pills under the name: that this is a lead, where it stands on the
// ladder, and who else may open it.
export function LeadPulse({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  const t = useT();
  const label = leadStatusLabel(lead.status);
  // The same row shape the contact's marks take: pills side by side with a
  // space between, never stretched across the head.
  return (
    <IdentityLine separator="space">
      {/* Plain, not a state colour: that this is a lead is a kind, not a
          reading. */}
      <Badge>{t("lead.marker")}</Badge>
      <Badge tone={statusPillTone(lead.status)}>
        {label ? t(label) : lead.status}
      </Badge>
      <LeadSharePill lead={lead} />
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
  field,
  roster,
  rosterPartial,
  candidates,
  meId,
}: Readonly<{
  field: FieldEditControl;
  roster: ReturnType<typeof useRoster>;
  rosterPartial: boolean;
  candidates: readonly Readonly<{ id: string; display_name?: string }>[];
  meId: string | undefined;
}>) {
  const t = useT();
  if (roster.isPending) {
    return <span>{t("share.rosterLoading")}</span>;
  }
  if (roster.isError) {
    return <RosterFailedLine kind="user" roster={roster} />;
  }
  // "Nobody else" is a claim about the WHOLE workspace, so only a roster read
  // to its end may make it. Over a walk that stopped early it would report a
  // lead as unassignable when the colleague to hand it to sits on a page
  // nothing here read.
  if (candidates.length === 0 && !rosterPartial) {
    return <span>{t("lead.assignNobodyElse")}</span>;
  }
  const { control, draft, setDraft, saving } = field;
  return (
    <Select
      {...control}
      placeholder={t("lead.assignChoose")}
      value={draft}
      disabled={saving}
      aria-busy={saving || undefined}
      options={candidates.map((entry) => ({
        value: entry.id,
        label: candidateLabel(entry, meId, t),
      }))}
      onChange={setDraft}
    />
  );
}

// Ownership: who holds the lead, and reassignment to any workspace user.
// The owner reads as a name. EntityRef resolves it by id and falls back to the
// id only once that read has settled without one. So a reader is never handed
// a bare uuid while a name is still on its way.
// Reassignment is a plain owner change. The server audits it and keeps
// whatever routing decision it overrides. So the only thing this control owes
// the reader is the list of who they can hand it to.
function LeadOwner({
  lead,
  meId,
  editable,
  onAssign,
}: Readonly<{
  lead: Lead;
  meId: string | undefined;
  editable: boolean;
  onAssign: (ownerId: string) => Promise<unknown>;
}>) {
  const t = useT();
  const [picking, setPicking] = useState(false);
  const roster = useRoster("user", picking);
  const rosterPartial = useRosterPartial("user", picking);
  const partialHint = useRosterPartialHint(rosterPartial);
  // Everyone but the current owner, with the viewer first: assigning to
  // yourself is the common case on a small team.
  const candidates = (roster.data ?? [])
    .filter(mayOwnRecords)
    .filter((entry) => entry.id !== lead.owner_id)
    .sort((a, b) => {
      if (a.id === meId) return -1;
      if (b.id === meId) return 1;
      return 0;
    });
  const label = t("lead.ownerLabel");
  let owner: ReactNode = t("lead.unassigned");
  if (lead.owner_id) {
    owner =
      lead.owner_id === meId ? (
        t("lead.ownerYou")
      ) : (
        <EntityRef kind="user" id={lead.owner_id} />
      );
  }
  return (
    <>
      <EditableValue
        editLabel={t("inlineChoice.change", { field: label })}
        editable={editable || picking}
        onEdit={() => setPicking(true)}
      >
        {owner}
      </EditableValue>
      {/* The draft starts unpicked: the current owner is no candidate, and
          Save on no pick is no write. */}
      <FieldEditDialog
        open={picking}
        label={label}
        value=""
        hint={partialHint}
        onClose={() => setPicking(false)}
        onSave={async (ownerId) => {
          await onAssign(ownerId);
        }}
      >
        {(field) => (
          <AssigneePicker
            field={field}
            roster={roster}
            rosterPartial={rosterPartial}
            candidates={candidates}
            meId={meId}
          />
        )}
      </FieldEditDialog>
    </>
  );
}

// The lead's owner, with the control that hands it on.
//
// Its own component rather than inline in the header's props, because what
// makes it pressable is three separate questions and they belong beside each
// other rather than spread through a JSX attribute list.
export function LeadOwnerControl({
  lead,
  writer,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
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
  return (
    <LeadOwner
      lead={lead}
      meId={me.data?.user?.id}
      editable={
        mayAssign &&
        !lead.archived_at &&
        !writer.patch.isPending &&
        !writer.claim.isPending
      }
      // Picking yourself on an unowned lead goes through the claim door: the
      // write arm refuses a PATCH to a row nobody owns.
      onAssign={(ownerId) =>
        !lead.owner_id && ownerId === me.data?.user?.id
          ? writer.claim.mutateAsync()
          : writer.saveAsync({ owner_id: ownerId })
      }
    />
  );
}

// What moved a system-set Engaged, in the few words the status row has.
const SET_BY_TRIGGER = {
  inbound_reply: "lead.ladder.setByReply",
  meeting_booked: "lead.ladder.setByBooking",
  meeting_held: "lead.ladder.setByMeeting",
} as const;

// How the system or a user put the lead on its rung, after the rung's name.
export function statusSetBy(lead: Lead, t: ReturnType<typeof useT>): string {
  const key = leadStatusLabel(lead.status);
  const label = key ? t(key) : lead.status;
  if (lead.status_set_by === "human") {
    return t("lead.ladder.byHand", { label });
  }
  if (lead.status_set_by !== "system") {
    return label;
  }
  const trigger = lead.qualification_evidence?.trigger;
  return trigger && lead.status === "engaged"
    ? t(SET_BY_TRIGGER[trigger], { label })
    : t("lead.ladder.automatic", { label });
}
