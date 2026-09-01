import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What happens to a mailbox's mail, said BEFORE the mailbox is connected.
//
// Every connect surface shows the same words: the two OAuth panels, the IMAP
// panel in onboarding, and the IMAP form in Settings. One component rather than
// four copies, because this is the sentence the DACH compliance package is
// about — a person is told what reading their mailbox means before they grant
// it, and four copies would drift until they disagreed about what was promised.
//
// It asks for NOTHING. There is no checkbox, no acknowledgement, and no field
// the server records: a mailbox connects with the notice unread. That is the
// product's posture and the handbook says so out loud — Margince tells the
// person what happens and does not verify that anyone read it. An
// acknowledgement here would be a consent record nobody actually gave, which is
// worse than none: `share_acknowledged_at` already carries that lesson, and its
// column comment says it records a grant time and not a consent.
//
// The claim it makes is true because of what ships around it: a new connection
// is born `classified` (the column default), so nothing this mailbox brings in
// is readable by a colleague until a classifier has judged the thread ordinary.
export function CaptureNotice() {
  const t = useT();
  return (
    <Callout tone="info">
      <p>{t("captureNotice.whatHappens")}</p>
      <p>{t("captureNotice.whoReads")}</p>
      <p>{t("captureNotice.yourControl")}</p>
    </Callout>
  );
}
