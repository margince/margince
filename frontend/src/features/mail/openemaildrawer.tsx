// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { CommitmentReview } from "../../design-system/commitmentreview";
import { EmailDetail } from "../../design-system/emaildetail";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { EmailAccessEditor } from "../capture/emailaccesseditor";
import { ComposeModal } from "./compose";
import { RelinkModal } from "./composerelink";
import { EmailRecordLinks } from "./emailrecords";
import { EmailDrawerActions, replyAnchor } from "./emailreply";
import { forwardSubject } from "./replysubject";

type Activity = components["schemas"]["Activity"];
type EmailPresentation = components["schemas"]["EmailPresentation"];

/**
 * The record page's one email drawer.
 *
 * A component rather than a conditional at each call site: five record pages
 * mount the same three lines, and `{openEmail && <EmailDetail …/>}` spends a
 * branch inside render callbacks that are already at the complexity ceiling.
 * Nothing is drawn when no message is open.
 *
 * Nothing is drawn until the first message is opened. From then on the drawer
 * stays mounted with `open` false, because that is what lets it animate out —
 * a drawer unmounted the moment its id goes away vanishes instead of leaving,
 * and takes the message off the screen the reader is still looking at.
 *
 * This is also where the access editor is bound, so every page that mounts the
 * drawer gets it. `EmailDetail` takes it as a render prop and never imports it:
 * the editor performs writes and reads the roster, which is screen work, and
 * the catalog component stays something a story can draw with no API behind it.
 *
 * The named records and the reply verb are bound here for the same reason and
 * with the same result: every surface that opens a message — a record page,
 * the worklist, a search hit, a brief's citation — gets the same drawer, so a
 * message can be answered and its records reached from wherever it was opened
 * rather than only from the timeline row a few of those surfaces have.
 */
export function OpenEmailDrawer({
  activityId,
  zone,
  onClose,
  onReplySent,
  activity,
}: Readonly<{
  activityId: string | null;
  /** The record's timezone, which the page owns. */
  zone: string;
  onClose: () => void;
  /**
   * A reply left the drawer, for a host whose own reading of the message the
   * send changes. A record page needs nothing: the composer refreshes the
   * record timelines it files under. The worklist's waiting lane does, or its
   * row goes on asking for a reply that has gone.
   */
  onReplySent?: () => void;
  /** The answer of a live read of this activity the host already holds, so
   *  the drawer does not read it a second time. Absent, the drawer reads it. */
  activity?: Activity;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const formatWhen = (iso: string) => formatDateTime(iso, locale, zone);
  // What the reader started from the drawer: a reply or a forward, in the
  // docked composer. The drawer steps aside while it is open, so the page and
  // the composer are both in view, and comes back when it closes.
  const [writing, setWriting] = useState<Writing | null>(null);
  const [relinking, setRelinking] = useState<EmailPresentation | null>(null);
  // The message whose thread a reply from here was added to.
  const [replied, setReplied] = useState<string | null>(null);
  // The id the drawer was opened with, kept after the host clears it: the
  // drawer is still on screen while it closes, and it has to name a message to
  // draw one. The next open replaces it.
  const [shown, setShown] = useState<string | null>(activityId);
  if (activityId !== null && activityId !== shown) {
    setShown(activityId);
    setReplied(null);
    setWriting(null);
  }
  if (shown === null) {
    return null;
  }
  return (
    <>
      <EmailDetail
        activityId={shown}
        open={activityId !== null && writing === null}
        onClose={onClose}
        formatWhen={formatWhen}
        notice={replied === shown ? t("email.thread.replied") : undefined}
        renderRelated={() => (
          <SourceCommitmentReview
            activityId={activityId ?? undefined}
            open={activityId !== null}
            zone={zone}
            given={
              activity !== undefined && activity.id === activityId
                ? activity
                : undefined
            }
          />
        )}
        renderAccess={(presentation) => (
          <EmailAccessEditor presentation={presentation} />
        )}
        renderRecords={(presentation) => (
          <EmailRecordLinks presentation={presentation} />
        )}
        renderActions={(presentation) => (
          <EmailDrawerActions
            presentation={presentation}
            onReply={() => setWriting({ kind: "reply", presentation })}
            onForward={() => setWriting({ kind: "forward", presentation })}
            onRelink={() => setRelinking(presentation)}
          />
        )}
      />
      {writing && (
        <DrawerComposer
          writing={writing}
          formatWhen={formatWhen}
          onClose={() => setWriting(null)}
          onSent={() => {
            if (writing.kind === "reply") {
              setReplied(shown);
              onReplySent?.();
            }
          }}
        />
      )}
      {relinking && (
        <DrawerRelink
          presentation={relinking}
          onClose={() => setRelinking(null)}
        />
      )}
    </>
  );
}

type Writing = Readonly<{
  kind: "reply" | "forward";
  presentation: EmailPresentation;
}>;

// The composer a drawer verb opens: a reply anchored to the message, or a new
// message carrying it forward. Either files under the message's own record.
function DrawerComposer({
  writing,
  formatWhen,
  onClose,
  onSent,
}: Readonly<{
  writing: Writing;
  formatWhen: (iso: string) => string;
  onClose: () => void;
  onSent: () => void;
}>) {
  const t = useT();
  const { presentation } = writing;
  const anchor = replyAnchor(presentation.links);
  if (!anchor) {
    return null;
  }
  const contactId =
    anchor.entity_type === "contact" ? anchor.entity_id : undefined;
  if (writing.kind === "reply") {
    return (
      <ComposeModal
        activityId={presentation.id}
        kind="email"
        entityType={anchor.entity_type}
        entityId={anchor.entity_id}
        contactId={contactId}
        open
        onClose={onClose}
        onSent={onSent}
      />
    );
  }
  return (
    <ComposeModal
      entityType={anchor.entity_type}
      entityId={anchor.entity_id}
      contactId={contactId}
      initialMessage={{
        subject: forwardSubject(presentation.summary.subject ?? ""),
        body: forwardedText(presentation, formatWhen, t),
      }}
      open
      onClose={onClose}
      onSent={onSent}
    />
  );
}

// The forwarded message under the reader's own words, with the header lines
// a mail client puts above it. Its files stay with the original.
export function forwardedText(
  presentation: EmailPresentation,
  formatWhen: (iso: string) => string,
  t: ReturnType<typeof useT>,
): string {
  const names = (parties: EmailPresentation["to"]) =>
    parties
      .map((party) =>
        party.display_name?.trim()
          ? `${party.display_name.trim()} <${party.address}>`
          : party.address,
      )
      .join(", ");
  const lines = [
    "",
    `---------- ${t("email.forwardHeader")} ----------`,
    `${t("email.detail.from")}: ${names(presentation.from)}`,
    `${t("email.detail.when")}: ${formatWhen(presentation.occurred_at)}`,
    `${t("compose.subject")}: ${presentation.summary.subject ?? ""}`,
    `${t("email.detail.to")}: ${names(presentation.to)}`,
  ];
  if (presentation.cc.length > 0) {
    lines.push(`${t("email.detail.cc")}: ${names(presentation.cc)}`);
  }
  return [...lines, "", presentation.body ?? ""].join("\n");
}

// Filing the message under another record, from the drawer's More menu.
function DrawerRelink({
  presentation,
  onClose,
}: Readonly<{ presentation: EmailPresentation; onClose: () => void }>) {
  const anchor = replyAnchor(presentation.links);
  if (!anchor) {
    return null;
  }
  return (
    <RelinkModal
      activityId={presentation.id}
      activityVersion={presentation.version}
      threadKey={presentation.thread_key}
      entityType={anchor.entity_type}
      entityId={anchor.entity_id}
      open
      onClose={onClose}
    />
  );
}

function SourceCommitmentReview({
  activityId,
  open,
  zone,
  given,
}: Readonly<{
  activityId: string | undefined;
  open: boolean;
  zone: string;
  given?: Activity;
}>) {
  const read = useActivity(activityId, open && given === undefined);
  const activity = given ?? read.data;
  const t = useT();
  if (!open || activity?.content_state === "withheld") return null;
  if (read.isError)
    return (
      <SurfaceState
        state="failed"
        emptyLabel={t("commitment.review.title")}
        loadingLabel={t("activity.detail.loading")}
        detail={{ onRetry: () => void read.refetch() }}
      >
        {null}
      </SurfaceState>
    );
  return (
    <CommitmentReview decisions={activity?.commitment_admissions} zone={zone} />
  );
}
