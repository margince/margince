// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { WorklistItem } from "./worklist.queries";

export function PromiseRecipients({
  recipients,
}: Readonly<{ recipients: WorklistItem["promise_recipients"] }>) {
  const t = useT();
  if (!recipients?.length) return null;
  return (
    <ul
      className="worklist-promise-recipients"
      aria-label={t("worklist.recipients.label")}
    >
      {recipients.map((recipient) => (
        <li key={recipient.claim_id}>
          <span>{recipient.display_name}</span>{" "}
          <Badge tone={recipient.status === "done" ? "success" : undefined}>
            {t(`worklist.recipients.${recipient.status}`)}
          </Badge>
        </li>
      ))}
    </ul>
  );
}
