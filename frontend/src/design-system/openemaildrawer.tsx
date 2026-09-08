// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateTime } from "../format/format";
import { useLocale } from "../i18n";
import { EmailAccessEditor } from "../screens/emailaccesseditor";
import { EmailRecordLinks } from "../screens/emailrecords";
import { EmailReplyAction } from "../screens/emailreply";
import { EmailDetail } from "./emaildetail";

/**
 * The record page's one email drawer.
 *
 * A component rather than a conditional at each call site: five record pages
 * mount the same three lines, and `{openEmail && <EmailDetail …/>}` spends a
 * branch inside render callbacks that are already at the complexity ceiling.
 * Nothing is drawn when no message is open.
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
}>) {
  const { locale } = useLocale();
  if (!activityId) {
    return null;
  }
  return (
    <EmailDetail
      activityId={activityId}
      onClose={onClose}
      formatWhen={(iso) => formatDateTime(iso, locale, zone)}
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
