// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatActivityWhen } from "../format/activitywhen";
import { useLocale, useT } from "../i18n";
import { useActivity } from "../lib/activityread";
import type { SourceOpenerFor } from "../lib/opensource";
import { ProblemError, problemCode } from "../lib/problem";
import { useRecordZone } from "../lib/recordzone";
import { Button } from "./atoms";
import { SourceEmailPanel } from "./sourceemailpanel";
import { type SectionState, SurfaceState } from "./surfacestate";

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
  openerFor,
}: Readonly<{
  activityId: string;
  /** The host's opener for the whole source (`useSourceOpener`). A source it
   *  cannot open gets no button. */
  openerFor: SourceOpenerFor;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The kind decides the form, so it is resolved before either is drawn. The
  // PLAIN activity read, not the email presentation: that endpoint refuses any
  // kind but `email`, so asking it first would 404 on every transcript.
  const query = useActivity(activityId);
  const source: Activity | undefined = query.data;
  const code =
    query.error instanceof ProblemError
      ? problemCode(query.error.problem)
      : null;
  const unavailable = code === "permission_denied" || code === "not_found";
  let state: SectionState = "ready";
  if (unavailable || source?.content_state === "withheld") state = "withheld";
  else if (query.isError) state = "failed";
  else if (!source) state = "loading";
  if (state !== "ready" || !source) {
    return (
      <SurfaceState
        label={t("stat.evidence")}
        labelLevel="h4"
        state={state}
        emptyLabel={t("email.detail.none")}
        loadingLabel={t("email.detail.loading")}
        detail={
          state === "failed"
            ? { onRetry: () => void query.refetch() }
            : { withheldReason: t("email.detail.withheldReason") }
        }
      >
        {null}
      </SurfaceState>
    );
  }
  const open = openerFor({
    id: source.id,
    kind: source.kind,
    contentState: source.content_state,
  });
  if (source.kind === "email") {
    return (
      <SourceEmailPanel
        activityId={activityId}
        formatWhen={(at, precision) =>
          formatActivityWhen(at, precision, locale, recordZone)
        }
        onOpenThread={open}
      />
    );
  }
  return (
    <div>
      <p className="t-caption">
        {[
          formatActivityWhen(
            source.occurred_at,
            source.occurred_precision,
            locale,
            recordZone,
          ),
          source.subject,
        ]
          .filter(Boolean)
          .join(" · ")}
      </p>
      {open && (
        <Button variant="ghost" aria-haspopup="dialog" onClick={open}>
          {t("tasks.openSource")}
        </Button>
      )}
    </div>
  );
}
