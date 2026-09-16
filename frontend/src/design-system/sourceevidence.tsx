// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "../screens/common";
import { Button } from "./atoms";
import { SourceEmailPanel } from "./sourceemailpanel";
import { SurfaceState } from "./surfacestate";

type Activity = components["schemas"]["Activity"];

/**
 * The evidence a task was read out of, in the form that evidence takes.
 *
 * An EMAIL is drawn in place: the reader is inside the task's drawer, and the
 * message is the thing they opened the task to check. It used to sit behind an
 * "Open original" button that put a second drawer over the first, so the one
 * fact the task rests on cost a click and covered the task while it was read.
 *
 * A MEETING TRANSCRIPT keeps the button. A transcript is long, it is a record
 * of its own with a subject and a date, and folding one into a task panel
 * inline would bury the task's own verbs under somebody else's hour.
 */
export function SourceEvidence({
  activityId,
  onOpenTranscript,
}: Readonly<{
  activityId: string;
  /** Open the transcript reader, which the host owns along with its drawer. */
  onOpenTranscript: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The kind decides the form, so it is resolved before either is drawn. The
  // PLAIN activity read, not the email presentation: that endpoint refuses any
  // kind but `email`, so asking it first would 404 on every transcript.
  const query = useQuery({
    queryKey: ["activity", activityId],
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}", {
        params: { path: { id: activityId } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
  });
  // A failed lookup is SAID, not swallowed. Returning nothing here removed the
  // evidence altogether — no panel, no button, no reason — and the email
  // panel's own failure arm cannot help, because a kind nobody resolved never
  // mounts it.
  if (query.isError) {
    return (
      <SurfaceState
        label={t("tasks.sourceEmail")}
        labelLevel="h4"
        state="failed"
        emptyLabel={t("email.detail.none")}
        loadingLabel={t("email.detail.loading")}
        detail={{ onRetry: () => void query.refetch() }}
      >
        {null}
      </SurfaceState>
    );
  }
  const source: Activity | undefined = query.data;
  // Nothing is drawn until the kind is known. A button that appeared and then
  // vanished as the read landed would offer a verb this task does not have.
  if (!source) {
    return null;
  }
  if (source.kind === "email") {
    return (
      <SourceEmailPanel
        activityId={activityId}
        formatWhen={(iso) => formatDateTime(iso, locale, recordZone)}
      />
    );
  }
  return (
    <div>
      <Button variant="ghost" onClick={() => onOpenTranscript(activityId)}>
        {t("tasks.openSource")}
      </Button>
    </div>
  );
}
