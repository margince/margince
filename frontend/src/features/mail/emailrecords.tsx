// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a message is filed against, named and reachable, on the drawer's
// envelope block.
//
// The presentation carries `links`. Without them a rep sees who a mail was
// with, and not which account, deal or project it belongs to.

import type { components } from "../../api/schema";
import { RecordLinks } from "../records/recordlinks";

type EmailPresentation = components["schemas"]["EmailPresentation"];

/**
 * The message's filed records, named and linked, or null when it is filed
 * against none: the drawer draws its label only over an answer.
 *
 * In the server's order, and in a new tab. The reference sits inside a dialog,
 * and following it in place would close the message being read.
 */
export function EmailRecordLinks({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  return <RecordLinks links={presentation.links} newTab />;
}
