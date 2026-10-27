// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { leadHeldOf } from "../lib/leadheld";
import { navigate } from "../lib/router";

// A contact form's answer when a live lead already holds the address or
// profile. A contact is never made beside a lead, so the reader converts the
// lead or opens it. Draws nothing for any other refusal.
export function LeadHeldNotice({
  error,
  onConvert,
  pending,
}: Readonly<{
  error: unknown;
  onConvert: (leadIds: string[]) => void;
  pending?: boolean;
}>) {
  const t = useT();
  const held = leadHeldOf(error);
  if (!held) return null;
  const [first] = held.leadIds;
  return (
    <Callout
      tone="warning"
      kind="outcome"
      title={t("contact.leadHeld.title")}
      actions={
        <>
          {held.convertible && (
            <Button
              type="button"
              variant="primary"
              pending={pending}
              onClick={() => onConvert(held.leadIds)}
            >
              {t("contact.leadHeld.convert")}
            </Button>
          )}
          {first && (
            <Button
              type="button"
              onClick={() => navigate({ screen: "leads", id: first })}
            >
              {t("contact.leadHeld.open")}
            </Button>
          )}
        </>
      }
    >
      {held.convertible
        ? t("contact.leadHeld.body")
        : t("contact.leadHeld.noConvert")}
    </Callout>
  );
}

// The notice for a form's refusal, or null when the refusal is something else,
// so the form keeps its ordinary error line. `resend` sends the same submit
// again with the extra body fields.
export function leadHeldNoticeFor(
  error: unknown,
  resend: (extra: Record<string, unknown>) => void,
  pending: boolean,
): ReactNode | null {
  if (!leadHeldOf(error)) return null;
  return (
    <LeadHeldNotice
      error={error}
      pending={pending}
      onConvert={(leadIds) => resend({ convert_lead_ids: leadIds })}
    />
  );
}
