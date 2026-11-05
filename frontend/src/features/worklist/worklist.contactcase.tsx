// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { ContactCaseReview } from "../../components/contactcase";
import { Button, Card } from "../../design-system/atoms";
import { INTL_LOCALE } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { WorklistItem } from "./worklist.queries";
import "./worklist.pair.css";

type AttentionContactCase = components["schemas"]["AttentionContactCase"];

function ownersKey(contactCase: AttentionContactCase): MessageKey {
  return contactCase.owners.length === 0
    ? "worklist.contactCase.unowned"
    : "worklist.contactCase.owners";
}

// The owners list says who the case belongs to, and some of them may hold
// only a read seat. So the authority line speaks of the reader alone.
function authorityKey(contactCase: AttentionContactCase): MessageKey {
  if (contactCase.can_merge) return "worklist.contactCase.yours";
  return contactCase.too_large
    ? "contactCase.notAllowed.tooLarge"
    : "worklist.contactCase.notYours";
}

/**
 * A group of contacts that may be duplicates, on its Worklist row. The row
 * names the members and their owners; the decision is made in the review.
 */
export function ContactCaseDecision({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const contactCase = item.contact_case;
  if (!contactCase) return null;
  const names = new Intl.ListFormat(INTL_LOCALE[locale], {
    style: "long",
    type: "conjunction",
  }).format(contactCase.owners);
  return (
    <div className="worklist-pair-review" data-testid="contact-case-row">
      <ul className="worklist-pair-cards">
        {contactCase.members.map((member) => (
          <Card as="li" inset key={member.id} className="worklist-pair-card">
            <span>{member.label}</span>
            {member.detail && (
              <span className="t-caption">{member.detail}</span>
            )}
          </Card>
        ))}
      </ul>
      <p className="t-caption worklist-pair-steward">
        {t(ownersKey(contactCase), { names })} {t(authorityKey(contactCase))}
      </p>
      <div className="worklist-pair-actions">
        <Button
          variant={contactCase.can_merge ? "primary" : "ghost"}
          onClick={() => setOpen(true)}
        >
          {t("worklist.contactCase.review")}
        </Button>
      </div>
      <ContactCaseReview
        contactId={contactCase.anchor_id}
        open={open}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}
