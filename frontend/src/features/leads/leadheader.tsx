// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead page's head, in the registers the contact page reads in: one fact
// per line under the name, then a pills row. Who holds the lead, and how it
// reached its status, are the details' Ownership rows.

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { OwnerPick } from "../../components/ownerpick";
import type { SectionFlow } from "../../components/sectionedit";
import { fetchGrants } from "../../components/share.parts";
import { Badge } from "../../design-system/atoms";
import { ReadOnlyValue } from "../../design-system/readonlyvalue";
import { useT } from "../../i18n";
import { useCanWrite, useWriteBlock } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { WRITE_BLOCK_REFUSAL } from "../../lib/writerefusal";
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
  // Bare pills, as the contact's marks are: the head's pill row spaces them.
  return (
    <>
      {/* Plain, not a state colour: that this is a lead is a kind, not a
          reading. */}
      <Badge>{t("lead.marker")}</Badge>
      <Badge tone={statusPillTone(lead.status)}>
        {label ? t(label) : lead.status}
      </Badge>
      <LeadSharePill lead={lead} />
    </>
  );
}

/** Who holds the lead. */
export function LeadOwnerName({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const meId = useMe().data?.user?.id;
  if (!lead.owner_id) {
    return t("lead.unassigned");
  }
  return lead.owner_id === meId ? (
    t("lead.ownerYou")
  ) : (
    <EntityRef kind="user" id={lead.owner_id} />
  );
}

// Not the row's `writable`: an unowned lead is not writable, and its claim is the
// way out of the unassigned queue. Grant and seat still bind; a closed lead does not move.
function useOwnerRefusal(lead: Lead, writer: LeadWriter) {
  const t = useT();
  const mayAssign = useCanWrite("lead", "update");
  const block = useWriteBlock("lead");
  if (lead.archived_at) {
    return { refused: true, reason: writer.readOnlyReason };
  }
  return {
    refused: !mayAssign,
    reason: block ? t(WRITE_BLOCK_REFUSAL[block]) : undefined,
  };
}

/** The Ownership row's reading, saying why it is fixed for a reader who may not hand it on. */
export function LeadOwnerReading({
  lead,
  writer,
  terminalReasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; terminalReasonId: string }>) {
  const { reason } = useOwnerRefusal(lead, writer);
  return (
    <ReadOnlyValue
      reason={reason || undefined}
      describedBy={lead.archived_at ? terminalReasonId : undefined}
    >
      <LeadOwnerName lead={lead} />
    </ReadOnlyValue>
  );
}

/** The owner handover as a flow of the Ownership section, with the claim of an unowned lead. */
export function useLeadOwnerFlow(
  lead: Lead,
  writer: LeadWriter,
): Readonly<{ flow?: SectionFlow; element: ReactNode }> {
  const t = useT();
  const meId = useMe().data?.user?.id;
  const [open, setOpen] = useState(false);
  const { refused } = useOwnerRefusal(lead, writer);
  if (refused) {
    return { element: null };
  }
  return {
    flow: {
      label: t("lead.ownerLabel"),
      reading: <LeadOwnerName lead={lead} />,
      open: () => setOpen(true),
    },
    element: (
      <OwnerPick
        opener="caller"
        open={open}
        onClose={() => setOpen(false)}
        record={lead}
        assign={(ownerId, opened) => writer.assign(ownerId, opened, meId)}
        invalidate="leads"
        recordKey="lead"
        pending={writer.patch.isPending}
      />
    ),
  };
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
