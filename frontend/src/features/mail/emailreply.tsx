// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the email drawer offers to do with the thread it shows: answer it,
// pass it on, or file it under another record. Each opens the app's own
// composer or relink dialog, so the drawer offers what the timeline offers.

import { Forward, Reply } from "lucide-react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { useT } from "../../i18n";
import type { RelinkKind } from "./composerelink";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type ActivityLink = components["schemas"]["ActivityLink"];

/**
 * Which of the message's records the reply is filed under.
 *
 * The composer anchors a send to ONE record — a reply written from a deal
 * timeline files under the deal and a reply written from a contact files under
 * the contact — and the drawer is opened from surfaces that are not a record
 * page at all, so it answers from the message's own filing instead of from
 * where it was opened.
 *
 * A contact leads because a reply is written TO a contact and their timeline is
 * where the conversation reads as a conversation. Where the message names no
 * contact, the work it is about; the account last, being the broadest thing
 * the message could be filed under.
 */
const ANCHOR_ORDER: readonly RelinkKind[] = [
  "contact",
  "deal",
  "lead",
  "project",
  "company",
];

/**
 * The message's own filing, read through the order above.
 *
 * Order in the LIST must not decide it: the server sends links in its own
 * order, and an anchor taken from that would move whenever the read's ordering
 * did. Null when the message is filed against nothing at all.
 */
export function replyAnchor(
  links: readonly ActivityLink[],
): ActivityLink | null {
  for (const kind of ANCHOR_ORDER) {
    const found = links.find((link) => link.entity_type === kind);
    if (found) {
      return found;
    }
  }
  return null;
}

/**
 * Actions for one expanded message: Reply, Forward, and the rest behind More.
 *
 * Reply follows the server's `can_reply`, so a withheld message offers none.
 * Every verb needs a record to file under. A message filed against nothing
 * offers none of them, since its send could show on no timeline.
 */
export function EmailDrawerActions({
  presentation,
  onReply,
  onForward,
  onRelink,
}: Readonly<{
  presentation: EmailPresentation;
  onReply: () => void;
  onForward: () => void;
  onRelink: () => void;
}>) {
  const t = useT();
  const anchor = replyAnchor(presentation.links);
  if (!anchor || presentation.access.content_state === "withheld") {
    return null;
  }
  return (
    <>
      {presentation.can_reply && (
        <Button variant="primary" onClick={onReply}>
          <Reply aria-hidden="true" />
          {t("compose.reply")}
        </Button>
      )}
      <Button variant="ghost" onClick={onForward}>
        <Forward aria-hidden="true" />
        {t("email.forward")}
      </Button>
      {presentation.can_relink && (
        <span className="emaildetail__more">
          <OverflowMenu label={t("record.moreActions")}>
            <Button onClick={onRelink}>{t("email.fileUnder")}</Button>
          </OverflowMenu>
        </span>
      )}
    </>
  );
}
