// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { formatDayMonth } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { worklistKey } from "../../lib/workkeys";

const followStateKey = (contactId: string) =>
  ["contact", contactId, "nudge-dismissal"] as const;

/**
 * The reader's own standing choice about a quiet contact, with Resume. The
 * Worklist's Undo lasts only as long as its toast; this is where a reader
 * finds the contact again after a reload. A colleague's choice never shows.
 */
export function ContactFollowState({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
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
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/contacts/{id}/nudge-dismissal", {
          params: { path: { id: contactId } },
        }),
        t,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: followStateKey(contactId) });
      queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
  const current = state.data;
  let title: string;
  if (current?.state === "stopped") {
    title = t("contact.follow.stopped");
  } else if (current?.state === "set_aside" && current.dismissed_until) {
    title = t("contact.follow.setAside", {
      date: formatDayMonth(current.dismissed_until, locale, zone),
    });
  } else {
    return null;
  }
  return (
    <Callout kind="standing" tone="info" title={title}>
      <Button pending={resume.isPending} onClick={() => resume.mutate()}>
        {t("contact.follow.resume")}
      </Button>
      <ErrorLine error={resume.error} inline />
    </Callout>
  );
}
