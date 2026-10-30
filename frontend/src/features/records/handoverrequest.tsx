// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Asking a colleague to hand over the contact or company they own.
//
// It sits in the access panel, under the refusal that names who owns the
// record, as the next step after it. The owner answers on their Worklist.
// This control only asks and takes the ask back.

import { Button } from "../../design-system/button";
import { ErrorLine } from "../../design-system/errorline";
import { useToast } from "../../design-system/toast";
import { formatDate } from "../../format/format.dates";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCan, useCanMutate } from "../../lib/capability";
import { useMemberName } from "../../lib/membernames";
import { problemMessageOf } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import {
  type TransferableRecordType,
  useOwnershipRequest,
  useOwnershipRequestActions,
} from "./ownershiprequest";

type HandoverRecord = Readonly<{
  id?: string;
  owner_id?: string | null;
  writable?: boolean;
  archived_at?: string | null;
}>;

/**
 * The ask, for a reader who could change this kind of record but not this one
 * because a colleague owns it. Nothing renders for anyone else.
 */
export function HandoverRequest({
  kind,
  record,
}: Readonly<{ kind: TransferableRecordType; record: HandoverRecord }>) {
  const viewerId = useViewerId();
  const granted = useCan(kind, "update");
  const mayHold = useCanMutate() && granted;
  const ownerId = record.owner_id ?? undefined;
  const asks =
    mayHold &&
    Boolean(record.id) &&
    ownerId !== undefined &&
    ownerId !== viewerId &&
    !record.writable &&
    !record.archived_at;
  const id = record.id ?? "";
  const open = useOwnershipRequest(kind, id, asks);
  const owner = useMemberName(asks ? ownerId : null);
  if (!asks || open.isPending || open.isError || !owner.data) {
    return null;
  }
  return (
    <HandoverAsk
      kind={kind}
      id={id}
      owner={owner.data}
      expiresAt={open.data?.expires_at}
    />
  );
}

function HandoverAsk({
  kind,
  id,
  owner,
  expiresAt,
}: Readonly<{
  kind: TransferableRecordType;
  id: string;
  owner: string;
  expiresAt: string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const { ask, withdraw } = useOwnershipRequestActions();
  const failure = ask.error ?? withdraw.error;
  if (expiresAt) {
    return (
      <div>
        <p className="t-caption">
          {t("handover.requested", {
            owner,
            date: formatDate(expiresAt, locale, viewerZone()),
          })}
        </p>
        <Button
          variant="link"
          pending={withdraw.isPending}
          onClick={() =>
            withdraw.mutate(
              { recordType: kind, id },
              { onSuccess: () => toast.show(t("handover.withdrawn")) },
            )
          }
        >
          {t("handover.withdraw")}
        </Button>
        {failure && (
          <ErrorLine inline>{problemMessageOf(failure, t)}</ErrorLine>
        )}
      </div>
    );
  }
  return (
    <div>
      <Button
        pending={ask.isPending}
        onClick={() =>
          ask.mutate(
            { recordType: kind, id },
            { onSuccess: () => toast.show(t("handover.sent", { owner })) },
          )
        }
      >
        {t("handover.ask", { owner })}
      </Button>
      {failure && <ErrorLine inline>{problemMessageOf(failure, t)}</ErrorLine>}
    </div>
  );
}
