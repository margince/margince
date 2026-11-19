// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead page's head, in the registers the contact page reads in: one fact
// per line under the name, then a pills row. Who holds the lead, and how it
// reached its status, are the details' Ownership rows.

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { OwnerPick } from "../../components/ownerpick";
import { fetchGrants } from "../../components/share.parts";
import { Badge } from "../../design-system/atoms";
import { IdentityLine } from "../../design-system/identityline";
import { ReadOnlyValue } from "../../design-system/readonlyvalue";
import { useT } from "../../i18n";
import { useCanWrite, useWriteRefusal } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
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

export function LeadOwnerControl({
  lead,
  writer,
  terminalReasonId,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  terminalReasonId: string;
}>) {
  const t = useT();
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
  const assignRefusal = useWriteRefusal("lead", "update", {
    seat: t("settings.seatRefused"),
    grant: t("record.actionRefused"),
  });
  const refused = Boolean(lead.archived_at) || !mayAssign;
  const meId = me.data?.user?.id;
  let owner: ReactNode = t("lead.unassigned");
  if (lead.owner_id) {
    owner =
      lead.owner_id === meId ? (
        t("lead.ownerYou")
      ) : (
        <EntityRef kind="user" id={lead.owner_id} />
      );
  }
  if (refused) {
    return (
      <ReadOnlyValue
        reason={
          lead.archived_at
            ? writer.readOnlyReason
            : (me.data && assignRefusal) || undefined
        }
        describedBy={lead.archived_at ? terminalReasonId : undefined}
      >
        {owner}
      </ReadOnlyValue>
    );
  }
  return (
    <>
      <span>{owner}</span>{" "}
      <OwnerPick
        record={lead}
        assign={(ownerId, opened) => writer.assign(ownerId, opened, meId)}
        invalidate="leads"
        recordKey="lead"
        pending={writer.patch.isPending}
      />
    </>
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
    ? t(labelFor(SET_BY_TRIGGER, trigger), { label })
    : t("lead.ladder.automatic", { label });
}
