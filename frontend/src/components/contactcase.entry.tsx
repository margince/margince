// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { navigate } from "../lib/router";
import { ContactCaseReview } from "./contactcase";
import { type ContactCase, useContactCase } from "./contactcase.queries";

/** How many other contacts an open case pairs this one with; 0 when it is in none. */
export function otherCaseMembers(contactCase: ContactCase | null | undefined) {
  if (contactCase?.status !== "open") return 0;
  return Math.max((contactCase.members?.length ?? 0) - 1, 0);
}

/**
 * The door to this contact's open duplicate case, drawn only while one exists.
 * A merge settles the case and so removes the door, which is why the merge is
 * confirmed in a toast. `followSurvivor` then opens the contact that remained.
 */
export function ContactCaseEntry({
  contactId,
  followSurvivor = false,
}: Readonly<{ contactId: string; followSurvivor?: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const contactCase = useContactCase(contactId).data;
  const others = otherCaseMembers(contactCase);
  if (others < 1) return null;
  // A case too large to list in full shows only some members. Their count is
  // then a floor, and the label says so with a plus.
  const count = contactCase?.too_large
    ? `${formatNumber(contactCase.members.length, locale)}+`
    : formatNumber(others, locale);
  return (
    <>
      <Button onClick={() => setOpen(true)} data-testid="contact-case-entry">
        {plural("contactCase.entry", others, { count })}
      </Button>
      <ContactCaseReview
        contactId={contactId}
        open={open}
        onClose={() => setOpen(false)}
        onMerged={(outcome) => {
          setOpen(false);
          toast.show(t("contactCase.merged"));
          if (followSurvivor && outcome.survivor_id !== contactId) {
            navigate({ screen: "contacts", id: outcome.survivor_id });
          }
        }}
      />
    </>
  );
}
