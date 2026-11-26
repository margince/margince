// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { CallAction } from "../design-system/callaction";
import { ContactLink } from "../design-system/contactlink";
import { primaryPhone } from "../format/contacturi";
import { type Translator, useT } from "../i18n";
import { useWriteTo } from "../lib/writeto";

type Contact = components["schemas"]["Contact"];
type EmailTarget = components["schemas"]["ActivityEmailTarget"];

/**
 * Draft email to a lead or a company. The lead's draft fills To with the lead's
 * address. The company's composer asks which of its contacts the mail is for.
 * Neither has an address to hand a mail client, so with no composer to open
 * there is no verb.
 */
export function RecordDraftStep({
  target,
  intent,
}: Readonly<{ target: EmailTarget; intent: string }>) {
  const t = useT();
  const reasonId = useId();
  const writeTo = useWriteTo();
  const refusal = refusalOf(target, t);
  if (refusal !== undefined) {
    return (
      <>
        <Button variant="ai" reasonId={reasonId}>
          {t("task.next.draftEmail")}
        </Button>
        <p id={reasonId} className="t-caption task-next-reason">
          {refusal}
        </p>
      </>
    );
  }
  if (!writeTo || target.entity_type === "contact") return null;
  return (
    <Button
      variant="ai"
      onClick={() =>
        writeTo({
          entityType: target.entity_type,
          entityId: target.entity_id,
          draftIntent: intent,
        })
      }
    >
      {t("task.next.draftEmail")}
    </Button>
  );
}

/** Why the server refuses the email, in the reader's words. */
export function refusalOf(
  target: EmailTarget,
  t: Translator,
): string | undefined {
  switch (target.refusal) {
    case "archived":
      return t("contact.rail.archivedReadOnly");
    case "lead_closed":
      return t("lead.terminalReadOnly");
    case "no_address":
      return target.entity_type === "lead"
        ? t("task.next.noLeadAddress")
        : t("task.next.noAddress");
    default:
      return undefined;
  }
}

export function CallVerb({
  contact,
}: Readonly<{ contact: Contact | undefined }>) {
  const t = useT();
  if (!contact || contact.archived_at) {
    return null;
  }
  return (
    <CallAction
      appearance="button"
      label={t("record.verb.call")}
      number={primaryPhone(contact.phones)?.phone}
      record={{ entityType: "contact", entityId: contact.id }}
    />
  );
}

export function EmailVerb({
  contact,
  intent,
  reasonId,
}: Readonly<{
  contact: Contact;
  intent: string;
  reasonId: string | undefined;
}>) {
  const t = useT();
  if (reasonId !== undefined) {
    return (
      <Button variant="ai" reasonId={reasonId}>
        {t("task.next.draftEmail")}
      </Button>
    );
  }
  return (
    <ContactLink
      kind="email"
      appearance="button"
      label={t("record.verb.email")}
      draft={{ label: t("task.next.draftEmail"), intent }}
      address={contact.primary_email ?? undefined}
      record={{ entityType: "contact", entityId: contact.id }}
    />
  );
}
