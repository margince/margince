// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { nudgeDismissalKey, worklistKey } from "../../lib/workkeys";

const followStateKey = (contactId: string) =>
  [...nudgeDismissalKey, contactId] as const;

/**
 * The reader's own set-aside or stop on a quiet contact, with the way back.
 * The Worklist's Undo lasts only as long as its toast, so this is where a
 * reader finds the choice again after a reload. A colleague's never shows.
 *
 * The date is the reader's own clock: it says when the contact returns to
 * their Worklist, not a date of the record. The full date, because a set-aside
 * of up to ninety days can end in the next year.
 */
export function ContactFollowState({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const state = useQuery({
    queryKey: followStateKey(contactId),
    queryFn: async () =>
      unwrap(
        await api.GET("/contacts/{id}/nudge-dismissal", {
          params: { path: { id: contactId } },
        }),
        t,
      ),
  });
  const resume = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/contacts/{id}/nudge-dismissal", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: (_answer, id) => {
      queryClient.invalidateQueries({ queryKey: followStateKey(id) });
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
  // A failed read draws nothing: the notice only adds a way back, and the
  // page's own read reports a contact the reader cannot reach.
  const current = state.data;
  let title: string;
  if (current?.state === "stopped") {
    title = t("contact.follow.stopped");
  } else if (current?.state === "set_aside" && current.dismissed_until) {
    title = t("contact.follow.setAside", {
      date: formatDate(current.dismissed_until, locale, viewerZone()),
    });
  } else {
    return null;
  }
  return (
    <Callout
      kind="standing"
      tone="info"
      title={title}
      actions={
        <Button
          pending={resume.isPending}
          onClick={() => resume.mutate(contactId)}
        >
          {t("contact.follow.resume")}
        </Button>
      }
    >
      <ErrorLine error={resume.error} inline />
    </Callout>
  );
}
