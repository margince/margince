// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { ContactCaseReview } from "../../components/contactcase";
import { useContactCase } from "../../components/contactcase.queries";
import { Button } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural } from "../../i18n";

/** The door to this contact's open duplicate case, drawn only while one exists. */
export function ContactCaseEntry({
  contactId,
}: Readonly<{ contactId: string }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const contactCase = useContactCase(contactId).data;
  const others = (contactCase?.members?.length ?? 0) - 1;
  if (contactCase?.status !== "open" || others < 1) return null;
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
