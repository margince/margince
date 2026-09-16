// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { emailDetailKey } from "./emaildetail";
import { EmailText } from "./emailtext";
import { SurfaceState } from "./surfacestate";
import "./sourceemailpanel.css";

// The message a task was read out of, IN the task — not behind a button that
// opens a second drawer over the first.
//
// `EmailDetail` is the canonical reading of a message and stays that: it is a
// `Modal`, and a reader who opened a task from their queue is already inside
// one. Opening the mail there put a drawer over a drawer and asked for a click
// to see the evidence the task exists because of, which is the one thing the
// reader came to check. This draws the same body in the panel they are already
// looking at.
//
// What it deliberately does NOT carry: the audience editor, the filed-record
// links, the reply verb. Those are `EmailDetail`'s, and they are writes — a
// reader who wants to act on the message opens it properly. This is evidence,
// read in place.

type EmailPresentation = components["schemas"]["EmailPresentation"];

/**
 * The email a task came from, read where the task is.
 *
 * The read is made HERE rather than handed in, for the reason `EmailDetail`
 * makes its own: what may be shown is an authorization result, and a caller
 * passing a presentation it fetched elsewhere would be passing an answer that
 * may already be stale about who may read this message. `source_activity_id` is
 * a reference and not a grant — the server decides again, under this caller's
 * own scope.
 */
export function SourceEmailPanel({
  activityId,
  formatWhen,
}: Readonly<{
  activityId: string;
  /** The caller owns the reader's timezone, so it owns the formatting. */
  formatWhen: (iso: string) => string;
}>) {
  const t = useT();
  const read = useQuery({
    // The same key `EmailDetail` reads under, so the audience writes that
    // invalidate one invalidate the other. No per-open segment here: this panel
    // has no open and shut — it is mounted with the task and dies with it, so
    // the hazard that segment exists for (a reopen repainting the last answer)
    // cannot arise.
    queryKey: emailDetailKey(activityId),
    // A message's content is an authorization result, not a value that ages.
    staleTime: 0,
    gcTime: 0,
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/activities/{id}/email-presentation",
        { params: { path: { id: activityId } } },
      );
      if (error) {
        throw error;
      }
      return data;
    },
  });

  if (read.isPending) {
    return (
      <SurfaceState
        label={t("tasks.sourceEmail")}
        labelLevel="h4"
        state="loading"
        emptyLabel={t("email.detail.none")}
        loadingLabel={t("email.detail.loading")}
      >
        {null}
      </SurfaceState>
    );
  }
  if (read.isError || !read.data) {
    // `failed` with a retry rather than `unavailable`: the read can be asked
    // again, and a failure with nothing to press is the same as being told the
    // message is not there.
    return (
      <SurfaceState
        label={t("tasks.sourceEmail")}
        labelLevel="h4"
        state="failed"
        emptyLabel={t("email.detail.none")}
        loadingLabel={t("email.detail.loading")}
        detail={{ onRetry: () => void read.refetch() }}
      >
        {null}
      </SurfaceState>
    );
  }
  return <SourceEmailBody presentation={read.data} formatWhen={formatWhen} />;
}

/**
 * The message itself.
 *
 * The withheld case returns EARLY and shares nothing below it — not the
 * subject, not the sender, not the date. Each is a fact about the message, and
 * a reader outside its audience is owed the fact that a message exists rather
 * than any of its contents. `EmailDetail` holds the same line for the same
 * reason; a second reading of a mail that relaxed it would be a way around the
 * first.
 */
function SourceEmailBody({
  presentation,
  formatWhen,
}: Readonly<{
  presentation: EmailPresentation;
  formatWhen: (iso: string) => string;
}>) {
  const t = useT();
  if (presentation.access.content_state === "withheld") {
    return (
      <SurfaceState
        label={t("tasks.sourceEmail")}
        labelLevel="h4"
        state="withheld"
        emptyLabel={t("email.detail.none")}
        loadingLabel={t("email.detail.loading")}
        // Why a message is limited describes what it is about, so the reason
        // names neither the audience nor this reader's seat.
        detail={{ withheldReason: t("email.detail.withheldReason") }}
      >
        {null}
      </SurfaceState>
    );
  }
  const subject = presentation.summary.subject?.trim() || t("email.noSubject");
  return (
    <section className="sourceemail">
      <h4 className="sourceemail__label">{t("tasks.sourceEmail")}</h4>
      <p className="sourceemail__subject">{subject}</p>
      <p className="sourceemail__when">
        {formatWhen(presentation.occurred_at)}
      </p>
      <div className="sourceemail__body">
        <EmailText body={presentation.body ?? ""} />
      </div>
    </section>
  );
}
