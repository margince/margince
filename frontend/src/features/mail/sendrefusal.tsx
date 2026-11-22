// What a rep is told when a send is refused outside its review.

import { AppLink } from "../../design-system/applink";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Refusal } from "./composerefusal";

// Older servers may still refuse footer omissions before creating a review.
// That response cannot support a one-message decision from this client.
export function SendRefusal({ refusal }: Readonly<{ refusal: Refusal }>) {
  const t = useT();
  if (refusal === "mailbox") {
    return (
      <ErrorLine
        actions={<ConnectionsLink label="compose.mailboxNotSendCapableGoto" />}
      >
        {t("compose.mailboxNotSendCapable")}
      </ErrorLine>
    );
  }
  if (refusal === "noMailbox") {
    return <NoMailboxLine />;
  }
  if (refusal === "legacyFooter") {
    return <ErrorLine>{t("compose.legacyFooter")}</ErrorLine>;
  }
  if (refusal === "sharedUnsubscribe") {
    return <ErrorLine>{t("compose.sharedUnsubscribeToken")}</ErrorLine>;
  }
  return null;
}

function ConnectionsLink({ label }: Readonly<{ label: MessageKey }>) {
  const t = useT();
  return (
    <AppLink href="#/settings/connections" className="link-button">
      {t(label)}
    </AppLink>
  );
}

/** The line for a send refused because the sender holds no mailbox. The
 *  composer draws it before a send when the roster already says so. */
export function NoMailboxLine({ standing }: Readonly<{ standing?: true }>) {
  const t = useT();
  return (
    <ErrorLine
      standing={standing}
      actions={<ConnectionsLink label="compose.mailboxNotConnectedGoto" />}
    >
      {t("compose.mailboxNotConnected")}
    </ErrorLine>
  );
}
