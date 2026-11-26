// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Asking a colleague about the record they own. The handover and the edit
// access request each draw this control with their own route and words.
//
// It sits under the refusal that names who owns the record, as the next step
// after it. The owner answers on their Worklist. This control only asks and
// takes the ask back.

import type { UseMutationResult } from "@tanstack/react-query";
import { Button } from "../design-system/button";
import { ErrorLine } from "../design-system/errorline";
import { useToast } from "../design-system/toast";
import { formatDate } from "../format/format.dates";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan, useCanMutate } from "../lib/capability";
import type { EntityKind } from "../lib/entity";
import { useMemberName } from "../lib/membernames";
import { problemMessageOf } from "../lib/problem";
import { useViewerId } from "../lib/session";
import { useViewerZone } from "../lib/viewerzone";

export type AskedRecord = Readonly<{
  id?: string;
  owner_id?: string | null;
  writable?: boolean;
  archived_at?: string | null;
}>;

/**
 * The owner to ask, for a reader who could change this kind of record but not
 * this one because a colleague owns it. Undefined for anyone else.
 */
export function useOwnerToAsk(kind: EntityKind, record: AskedRecord) {
  const viewerId = useViewerId();
  const granted = useCan(kind, "update");
  const mayHold = useCanMutate() && granted;
  const ownerId = record.owner_id ?? undefined;
  const asks =
    mayHold &&
    Boolean(record.id) &&
    ownerId !== viewerId &&
    !record.writable &&
    !record.archived_at;
  return asks ? ownerId : undefined;
}

export type AskWords = Readonly<{
  ask: MessageKey;
  requested: MessageKey;
  sent: MessageKey;
  withdraw: MessageKey;
  withdrawn: MessageKey;
}>;

type Target<K> = Readonly<{ recordType: K; id: string }>;

type OpenRequest = Readonly<{
  isPending: boolean;
  isError: boolean;
  data?: Readonly<{ expires_at: string }> | null;
}>;

type AskActions<K> = Readonly<{
  ask: UseMutationResult<unknown, Error, Target<K>>;
  withdraw: UseMutationResult<void, Error, Target<K>>;
}>;

// Nothing renders until the open request and the owner's name are read. So the
// button does not turn from Ask into Withdraw under the pointer.
export function OwnerAsk<K extends string>({
  kind,
  id,
  ownerId,
  open,
  actions,
  words,
  variant,
}: Readonly<{
  kind: K;
  id: string;
  ownerId: string | undefined;
  open: OpenRequest;
  actions: AskActions<K>;
  words: AskWords;
  variant: "primary" | "ghost" | "link";
}>) {
  const owner = useMemberName(ownerId ?? null);
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  if (!ownerId || open.isPending || open.isError || !owner.data) {
    return null;
  }
  const ownerName = owner.data;
  const { ask, withdraw } = actions;
  const failure = ask.error ?? withdraw.error;
  const target = { recordType: kind, id };
  const expiresAt = open.data?.expires_at;
  if (expiresAt) {
    return (
      <div>
        <p className="t-caption">
          {t(words.requested, {
            owner: ownerName,
            date: formatDate(expiresAt, locale, readerZone),
          })}
        </p>
        <Button
          variant="link"
          pending={withdraw.isPending}
          onClick={() =>
            withdraw.mutate(target, {
              onSuccess: () => toast.show(t(words.withdrawn)),
            })
          }
        >
          {t(words.withdraw)}
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
        variant={variant}
        pending={ask.isPending}
        onClick={() =>
          ask.mutate(target, {
            onSuccess: () => toast.show(t(words.sent, { owner: ownerName })),
          })
        }
      >
        {t(words.ask, { owner: ownerName })}
      </Button>
      {failure && <ErrorLine inline>{problemMessageOf(failure, t)}</ErrorLine>}
    </div>
  );
}
