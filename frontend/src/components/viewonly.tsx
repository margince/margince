// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A record this reader may view but not change says so in two places on every
// record type. One is a "View only" pill in the head; one is an Access row in
// Ownership. Both carry the ask for edit access. The refusal sentence sits once,
// hidden, under the id every locked control points at.

import type { ReactNode } from "react";
import { Avatar } from "../design-system/avatar";
import { Popover } from "../design-system/popover";
import type { DetailAccess } from "../design-system/recorddetails";
import { VisibilityBadge } from "../design-system/visibility";
import { useT } from "../i18n";
import type { EditAccessRecordType } from "../lib/editaccessapi";
import { useMemberName } from "../lib/membernames";
import { useViewerId } from "../lib/session";
import { useKindWriteRefusal, useMeSettled } from "../lib/writerefusal";
import { EditAccessRequest } from "./editaccessrequest";
import { HandoverRequest } from "./handoverrequest";
import type { AskedRecord } from "./ownerask";
import "./viewonly.css";

type ViewOnlyProps = Readonly<{
  kind: EditAccessRecordType;
  record: AskedRecord | undefined;
  // The record's own name, as its head shows it.
  name: string;
  // The id every control this record refuses points at.
  reasonId: string;
}>;

/**
 * The head's pill, which opens who owns the record and the ask. Nothing for a
 * reader who may change it, and nothing for an archived record, whose band
 * already says why it takes no change.
 */
export function ViewOnly({ kind, record, name, reasonId }: ViewOnlyProps) {
  const t = useT();
  const reason = useViewOnlyReason(kind, record);
  if (!record || reason === undefined) {
    return null;
  }
  return (
    <>
      <span id={reasonId} className="sr-only">
        {reason}
      </span>
      <Popover
        dialog
        label={
          <>
            <span className="sr-only">{t("viewOnly.label")}: </span>
            <VisibilityBadge state="viewOnly" opens />
          </>
        }
      >
        <div className="view-only-panel">
          <OwnerLine record={record} name={name} />
          <p className="t-caption">{reason}</p>
          <EditAccessRequest kind={kind} record={record} />
          {(kind === "contact" || kind === "company") && (
            <HandoverRequest kind={kind} record={record} />
          )}
        </div>
      </Popover>
    </>
  );
}

/**
 * The Ownership group's Access row and its ask, or undefined for a reader who
 * may change the record. The ask is a ghost here: the head's panel carries the
 * filled one.
 */
export function useViewOnlyAccess(
  kind: EditAccessRecordType,
  record: AskedRecord | undefined,
): DetailAccess | undefined {
  const t = useT();
  const reason = useViewOnlyReason(kind, record);
  if (!record || reason === undefined) {
    return undefined;
  }
  return {
    row: {
      key: "access",
      term: t("record.details.access"),
      value: <VisibilityBadge state="viewOnly" />,
    },
    foot: <EditAccessRequest kind={kind} record={record} variant="ghost" />,
  };
}

// The page's own write rule (grant, seat, row), with the archive left to the
// archived band. Nothing until `/me` answers, so a writable record never flashes it.
function useViewOnlyReason(
  kind: EditAccessRecordType,
  record: AskedRecord | undefined,
): string | undefined {
  const reason = useKindWriteRefusal(kind, record);
  const settled = useMeSettled();
  return !settled || record?.archived_at ? undefined : reason;
}

function OwnerLine({
  record,
  name,
}: Readonly<{ record: AskedRecord; name: string }>): ReactNode {
  const t = useT();
  const viewerId = useViewerId();
  const ownerId = record.owner_id ?? null;
  const owner = useMemberName(ownerId);
  if (!ownerId || ownerId === viewerId || !owner.data) {
    return null;
  }
  return (
    <div className="view-only-owner">
      <Avatar name={owner.data} identity={ownerId} />
      <strong>{t("viewOnly.owns", { owner: owner.data, name })}</strong>
    </div>
  );
}
