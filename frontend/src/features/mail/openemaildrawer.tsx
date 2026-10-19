// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef } from "react";
import type { components } from "../../api/schema";
import { CommitmentReview } from "../../design-system/commitmentreview";
import { EmailDetail } from "../../design-system/emaildetail";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { EmailAccessEditor } from "../capture/emailaccesseditor";
import { EmailRecordLinks } from "./emailrecords";
import { EmailReplyAction } from "./emailreply";

type Activity = components["schemas"]["Activity"];

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
   * send changes.
   *
   * A record page needs nothing: the composer refreshes the record timelines
   * it files under. A page listing the UNANSWERED does — the worklist's row
   * verb already invalidates its queue on send, and a drawer that answered the
   * same message without doing so left the row still asking for a reply that
   * had gone.
   */
  onReplySent?: () => void;
  /** The answer of a live read of this activity the host already holds, so
   *  the drawer does not read it a second time. Absent, the drawer reads it. */
  activity?: Activity;
}>) {
  const { locale } = useLocale();
  // The id the drawer was opened with, kept after the host clears it: the
  // drawer is still on screen while it closes, and it has to name a message to
  // draw one. The next open replaces it.
  const shown = useRef<string | null>(null);
  if (activityId !== null) {
    shown.current = activityId;
  }
  if (shown.current === null) {
    return null;
  }
  return (
    <EmailDetail
      activityId={shown.current}
      open={activityId !== null}
      onClose={onClose}
      formatWhen={(iso) => formatDateTime(iso, locale, zone)}
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
      renderReply={(presentation) => (
        <EmailReplyAction presentation={presentation} onSent={onReplySent} />
      )}
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
