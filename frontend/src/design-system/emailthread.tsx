// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronUp, CircleCheck } from "lucide-react";
import { type ReactNode, useState } from "react";

import type { components } from "../api/schema";
import { useT } from "../i18n";
import { Avatar } from "./avatar";
import { Callout } from "./callout";
import { EmailEntry } from "./emailentry";
import {
  EmailAttachments,
  EmailEnvelope,
  emailDetailKey,
  partyName,
  readEmailPresentation,
} from "./emailmessage";
import { EmailText } from "./emailtext";
import { SurfaceState } from "./surfacestate";
import "./emailthread.css";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailSummary = components["schemas"]["EmailSummary"];

type Renders = Readonly<{
  formatWhen: (iso: string) => string;
  /** Names the records a message is filed against, on its envelope. */
  renderRecords?: (presentation: EmailPresentation) => ReactNode;
}>;

/**
 * A conversation read in order, oldest first, inside the email drawer.
 *
 * The message the reader opened is open; the rest of the thread is folded to
 * one `EmailEntry` row each. Opening a row reads that message on its own, so
 * the server decides again whether this reader may see it. `notice` is a line
 * under the last message, such as the word that a reply was added.
 */
export function EmailThread({
  presentation,
  formatWhen,
  renderRecords,
  related,
  notice,
}: Renders &
  Readonly<{
    presentation: EmailPresentation;
    /** What the opened message is the source of, drawn under it. */
    related?: () => ReactNode;
    notice?: string;
  }>) {
  const [opened, setOpened] = useState<ReadonlySet<string>>(
    () => new Set([presentation.id]),
  );
  const toggle = (id: string) =>
    setOpened((was) => {
      const next = new Set(was);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  return (
    <div className="emailthread">
      <ol className="emailthread__list">
        {threadOrder(presentation).map((summary) => {
          const isOpen = opened.has(summary.activity_id);
          const self = summary.activity_id === presentation.id;
          return (
            <li key={summary.activity_id}>
              {!isOpen && (
                <EmailEntry
                  summary={summary}
                  timestamp={formatWhen(summary.occurred_at)}
                  onSelect={() => toggle(summary.activity_id)}
                  selected={false}
                />
              )}
              {isOpen && self && (
                <OpenMessage
                  presentation={presentation}
                  formatWhen={formatWhen}
                  renderRecords={renderRecords}
                  related={related}
                  onFold={() => toggle(summary.activity_id)}
                />
              )}
              {isOpen && !self && (
                <ReadMessage
                  activityId={summary.activity_id}
                  formatWhen={formatWhen}
                  renderRecords={renderRecords}
                  onFold={() => toggle(summary.activity_id)}
                />
              )}
            </li>
          );
        })}
      </ol>
      {notice && (
        <p role="status" className="emailthread__notice">
          <CircleCheck aria-hidden="true" />
          {notice}
        </p>
      )}
    </div>
  );
}

/**
 * The opened message and the page of the thread the read carried, oldest
 * first. The opened one is in the list once, whatever the page holds.
 */
export function threadOrder(
  presentation: EmailPresentation,
): readonly EmailSummary[] {
  const others = (presentation.thread?.members ?? []).filter(
    (member) => member.activity_id !== presentation.id,
  );
  return [...others, presentation.summary].sort(
    (a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at),
  );
}

// Another message of the thread, read when the reader opens it.
function ReadMessage({
  activityId,
  formatWhen,
  renderRecords,
  onFold,
}: Renders & Readonly<{ activityId: string; onFold: () => void }>) {
  const t = useT();
  const read = useQuery({
    // Under the drawer's key, so the audience writes refresh it too.
    queryKey: [...emailDetailKey(activityId), "thread"],
    queryFn: () => readEmailPresentation(activityId),
    staleTime: 0,
    gcTime: 0,
  });
  if (read.data) {
    return (
      <OpenMessage
        presentation={read.data}
        formatWhen={formatWhen}
        renderRecords={renderRecords}
        onFold={onFold}
      />
    );
  }
  return (
    <SurfaceState
      state={read.isError ? "failed" : "loading"}
      emptyLabel={t("email.detail.none")}
      loadingLabel={t("email.detail.loading")}
      detail={{ onRetry: () => void read.refetch() }}
    >
      {null}
    </SurfaceState>
  );
}

/**
 * One message read whole: who wrote it and when, a line that opens its
 * envelope, its words and its files. A withheld message says so and shows
 * nothing else of itself.
 */
function OpenMessage({
  presentation,
  formatWhen,
  renderRecords,
  related,
  onFold,
}: Renders &
  Readonly<{
    presentation: EmailPresentation;
    related?: () => ReactNode;
    onFold: () => void;
  }>) {
  const t = useT();
  const [envelope, setEnvelope] = useState(false);
  if (presentation.access.content_state === "withheld") {
    return (
      <div className="emailthread__message">
        <SurfaceState
          loadingLabel={t("email.detail.loading")}
          state="withheld"
          emptyLabel={t("email.detail.none")}
          detail={{ withheldReason: t("email.detail.withheldReason") }}
        >
          {null}
        </SurfaceState>
      </div>
    );
  }
  const sender = presentation.from[0];
  const name = sender ? partyName(sender) : t("email.detail.none");
  const toNames = presentation.to.map(partyName).filter(Boolean).join(", ");
  return (
    <article className="emailthread__message" aria-label={name}>
      <div className="emailthread__head">
        <button
          type="button"
          className="emailthread__fold"
          aria-expanded="true"
          aria-label={t("email.thread.fold", { name })}
          onClick={onFold}
        >
          <Avatar name={name} identity={sender?.address ?? name} size="sm" />
          <span className="emailthread__who">
            <span className="emailthread__name">{name}</span>
            {sender && sender.address !== name && (
              <span className="t-caption">{sender.address}</span>
            )}
          </span>
          <span className="t-caption">
            {formatWhen(presentation.occurred_at)}
          </span>
        </button>
        <button
          type="button"
          className="emailthread__envelope t-caption"
          aria-expanded={envelope}
          onClick={() => setEnvelope((was) => !was)}
        >
          {toNames
            ? t("email.thread.toLine", { names: toNames })
            : t("email.thread.details")}
          {envelope ? (
            <ChevronUp aria-hidden="true" />
          ) : (
            <ChevronDown aria-hidden="true" />
          )}
        </button>
      </div>
      {envelope && (
        <EmailEnvelope presentation={presentation} formatWhen={formatWhen}>
          <FiledUnder presentation={presentation} render={renderRecords} />
        </EmailEnvelope>
      )}
      <TextNotice completeness={presentation.body_completeness} />
      <EmailText body={presentation.body ?? ""} />
      <EmailAttachments files={presentation.attachments} />
      {related?.()}
    </article>
  );
}

const TEXT_NOTICES = {
  partial: "email.thread.textPartial",
  unreadable: "email.thread.textUnreadable",
  forwarded: "email.thread.textForwarded",
  unassessed: "email.thread.textUnassessed",
} as const;

/**
 * Says that a mail's text was read only in part, not at all, not yet checked,
 * or is a forwarded original's.
 */
function TextNotice({
  completeness,
}: Readonly<{ completeness?: EmailPresentation["body_completeness"] }>) {
  const t = useT();
  if (completeness === "forwarded") {
    return <Callout kind="standing" title={t(TEXT_NOTICES.forwarded)} />;
  }
  if (
    completeness !== "partial" &&
    completeness !== "unreadable" &&
    completeness !== "unassessed"
  ) {
    return null;
  }
  return (
    <Callout kind="standing" title={t(TEXT_NOTICES[completeness])}>
      {t("email.thread.textUnreadableHint")}
    </Callout>
  );
}

/**
 * The records a message is filed against, on its envelope. The label shows
 * only when the presentation has links. A render prop's element is always
 * truthy, so it cannot tell.
 */
function FiledUnder({
  presentation,
  render,
}: Readonly<{
  presentation: EmailPresentation;
  render?: (presentation: EmailPresentation) => ReactNode;
}>) {
  const t = useT();
  if (!render || presentation.links.length === 0) {
    return null;
  }
  return (
    <p className="emaildetail__party">
      <span className="emaildetail__partyLabel">
        {t("email.detail.filedUnder")}
      </span>
      {render(presentation)}
    </p>
  );
}
