// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, usePlural } from "../i18n";
import { ContactCaseReview } from "./contactcase";
import { type ContactCase, useContactCase } from "./contactcase.queries";

/** How many other contacts an open case pairs this one with; 0 when it is in none. */
export function otherCaseMembers(contactCase: ContactCase | null | undefined) {
  if (contactCase?.status !== "open") return 0;
  return Math.max((contactCase.members?.length ?? 0) - 1, 0);
}

/** The door to this contact's open duplicate case, drawn only while one exists. */
export function ContactCaseEntry({
  contactId,
}: Readonly<{ contactId: string }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const others = otherCaseMembers(useContactCase(contactId).data);
  if (others < 1) return null;
  return (
    <>
      <Button onClick={() => setOpen(true)} data-testid="contact-case-entry">
        {plural("contactCase.entry", others, {
          count: formatNumber(others, locale),
        })}
      </Button>
      <ContactCaseReview
        contactId={contactId}
        open={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}
