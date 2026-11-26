// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { WEBSITE_INQUIRY_RECIPE } from "../../lib/websiteinquiries";

export function AutomationRecipientSummary({
  automation,
  action,
}: Readonly<{
  automation: components["schemas"]["Automation"];
  action: string;
}>) {
  const t = useT();
  if (automation.key === WEBSITE_INQUIRY_RECIPE) return null;
  if (action !== "create_task" && action !== "notify") return null;
  const authorizer =
    automation.params?.recipient === "authorizer" ||
    automation.key === "list_membership_notify";
  const notices = action === "notify";
  if (authorizer && automation.owner_id) {
    return (
      <span className="t-caption">
        {t(notices ? "auto.recipient.notices" : "auto.recipient.tasks")}{" "}
        <EntityRef kind="user" id={automation.owner_id} />
      </span>
    );
  }
  // A whole sentence each, so the words after "go to" keep their own case.
  let sentence: MessageKey = notices
    ? "auto.recipient.noticesToOwner"
    : "auto.recipient.tasksToOwner";
  if (authorizer) {
    sentence = notices
      ? "auto.recipient.noticesToAuthorizer"
      : "auto.recipient.tasksToAuthorizer";
  }
  return <span className="t-caption">{t(sentence)}</span>;
}
