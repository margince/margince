// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { contextOptions } from "../../lib/compose-context";
import "./compose.css";

type CommunicationContext = components["schemas"]["CommunicationContext"];

export type ReasonTarget = Readonly<{
  contactId: string;
  address: string;
  category: CommunicationContext;
}>;

type Preview = components["schemas"]["SendAuthorizationPreview"];

/**
 * The reason the record already carries for every recipient, when the claimed
 * one refuses them all. The engine names alternatives only for a recipient it
 * refuses. A switch changes the reason for the whole message, so a send with
 * an allowed recipient is offered none. An answer still out, or one that
 * failed, offers none either.
 */
export function alternativeReasonOf(permission: {
  preview: Preview | undefined;
  asking: boolean;
  unanswered: boolean;
}): CommunicationContext | undefined {
  if (permission.asking || permission.unanswered || !permission.preview) {
    return undefined;
  }
  const { recipients } = permission.preview;
  if (!recipients.every((r) => r.would_refuse)) {
    return undefined;
  }
  const [first, ...rest] = recipients;
  return (first?.supported_alternatives ?? []).find((reason) =>
    rest.every((r) => r.supported_alternatives?.includes(reason)),
  );
}

/**
 * The one press that answers a refused reason with the one the record
 * carries. It switches the reason and the engine is asked again; it records
 * nothing and sends nothing.
 */
export function ReasonSwitch({
  reason,
  onSwitch,
}: Readonly<{
  reason: CommunicationContext;
  onSwitch: (next: CommunicationContext) => void;
}>) {
  const t = useT();
  const label =
    contextOptions(t).find((option) => option.value === reason)?.label ??
    reason;
  return (
    <div className="compose-reason">
      <p className="t-caption">{t("compose.reasonSwitchText")}</p>
      <Button onClick={() => onSwitch(reason)}>
        {t("compose.reasonSwitch", { reason: label })}
      </Button>
    </div>
  );
}
