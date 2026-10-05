// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import {
  draftKey,
  readDraft,
  type SavedDraftAnchor,
} from "./composesaveddraft";

// A saved draft waiting on a record's page: one the reader's agent left for
// review through draft_email, or one the reader saved and closed.

type MailDraft = components["schemas"]["MailDraft"];

/**
 * The reader's saved draft for one record, which the record's page shows as
 * waiting. Read under the composer's own key and reader, so a save, a send or
 * a delete in the composer updates the page.
 *
 * Read again on mount and on focus, because an agent can leave a draft after
 * the page loaded. Not while the composer is open: it pins the version it
 * read, and a refetch under it would move that version without the 409 that
 * is the only warning the reader gets.
 */
export function useWaitingDraft(
  anchor: SavedDraftAnchor,
  composerOpen: boolean,
) {
  return useQuery({
    queryKey: draftKey(anchor),
    queryFn: () => readDraft(anchor),
    enabled: !composerOpen,
    staleTime: 0,
    refetchOnWindowFocus: true,
  });
}

/**
 * A draft waiting on the record: one the reader's agent left for review, or
 * one the reader saved and closed. Opening it opens the composer, which puts
 * the saved words back.
 */
export function WaitingDraftNotice({
  draft,
  onOpen,
}: Readonly<{ draft: MailDraft; onOpen: () => void }>) {
  const t = useT();
  const subject = draft.subject.trim() || t("compose.waitingDraftNoSubject");
  return (
    <Callout
      tone={draft.agent_drafted ? "ai" : "info"}
      kind="standing"
      title={t("compose.waitingDraftTitle")}
      actions={
        <Button variant="primary" onClick={onOpen}>
          {t("compose.waitingDraftOpen")}
        </Button>
      }
    >
      {t(
        draft.agent_drafted
          ? "compose.waitingDraftByAgent"
          : "compose.waitingDraftByYou",
        { subject },
      )}
    </Callout>
  );
}

/**
 * A record page's band: why the record takes no changes, then the draft
 * waiting there. Absent when there is neither: a line always reserved would
 * read as a record with something to say about itself and nothing said.
 */
export function useWaitingDraftBand(input: {
  anchor: SavedDraftAnchor;
  composerOpen: boolean;
  onOpen: () => void;
  readOnlyReason: string | null | undefined;
  readOnlyReasonId: string;
}): ReactNode {
  const waiting = useWaitingDraft(input.anchor, input.composerOpen).data;
  if (!input.readOnlyReason && !waiting) return undefined;
  return (
    <>
      {input.readOnlyReason && (
        <p id={input.readOnlyReasonId} className="t-caption">
          {input.readOnlyReason}
        </p>
      )}
      {waiting && <WaitingDraftNotice draft={waiting} onOpen={input.onOpen} />}
    </>
  );
}
