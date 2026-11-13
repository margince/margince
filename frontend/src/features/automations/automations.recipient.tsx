// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { useT } from "../../i18n";
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
  return (
    <span className="t-caption">
      {t(
        action === "notify" ? "auto.recipient.notices" : "auto.recipient.tasks",
      )}{" "}
      {authorizer ? (
        automation.owner_id ? (
          <EntityRef kind="user" id={automation.owner_id} />
        ) : (
          t("auto.ownerNone")
        )
      ) : (
        t("auto.renewal.recordOwner")
      )}
    </span>
  );
}
