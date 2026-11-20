// What a rep is told when a send is refused outside its review.

import { AppLink } from "../../design-system/applink";
import { useT } from "../../i18n";
import type { Refusal } from "./composerefusal";

// Older servers may still refuse footer omissions before creating a review.
// That response cannot support a one-message decision from this client.
export function SendRefusal({ refusal }: Readonly<{ refusal: Refusal }>) {
  const t = useT();
  if (refusal === "mailbox") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.mailboxNotSendCapable")}</p>
        <AppLink href="#/settings/connections" className="link-button">
          {t("compose.mailboxNotSendCapableGoto")}
        </AppLink>
      </div>
    );
  }
  if (refusal === "legacyFooter") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.legacyFooter")}</p>
      </div>
    );
  }
  if (refusal === "sharedUnsubscribe") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.sharedUnsubscribeToken")}</p>
      </div>
    );
  }
  return null;
}
