// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import type { components } from "../api/schema";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import type { LeadWriter } from "./leads";
import { leadFillsFrom, searchContacts } from "./leads.contactoffers";

type Lead = components["schemas"]["Lead"];
type Contact = components["schemas"]["Contact"];

/**
 * "Fill from a contact": the lead's name, address, title and company taken
 * from a contact the CRM already holds, in one save. It is how a lead created
 * against a company — drawn as "Unnamed lead" — gets its person without the
 * seller retyping somebody the CRM knows. Picking IS the act, so the picker
 * writes; what the contact does not hold is left as the lead has it.
 */
export function LeadFillFromContact({
  lead,
  writer,
  reasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; reasonId?: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // The rows a search answered, by id, so a pick can read the whole contact
  // rather than the name its row was drawn with.
  const found = useRef(new Map<string, Contact>());
  // THIS write's outcome, as the project picker beside it reads its own: the
  // mutation is shared with every other save on the page.
  const thisWrite =
    writer.patch.variables?.body !== undefined &&
    "full_name" in writer.patch.variables.body &&
    "title" in writer.patch.variables.body;
  const saved = writer.patch.isSuccess && thisWrite;
  const failed = writer.patch.isError && thisWrite;
  useEffect(() => {
    if (saved) {
      setOpen(false);
    }
  }, [saved]);
  if (writer.readOnly) {
    return null;
  }
  const label = t("lead.fillFromContact");
  return (
    <div className="card-actions">
      <ListPopover
        label={label}
        title={label}
        searchLabel={t("lead.fillFromContactSearch")}
        reasonId={reasonId}
        open={open}
        onOpenChange={(next) => {
          if (next || !writer.patch.isPending) {
            setOpen(next);
          }
        }}
        search={async (term): Promise<readonly ListPopoverOption[]> => {
          const contacts = await searchContacts(term);
          for (const contact of contacts) {
            found.current.set(contact.id, contact);
          }
          return contacts.map((contact) => ({
            id: contact.id,
            name: contact.full_name,
            hint: contact.primary_email ?? undefined,
            keywords: contact.primary_email ? [contact.primary_email] : [],
          }));
        }}
        onPick={(option) => {
          const contact = found.current.get(option.id);
          if (!contact) {
            return;
          }
          const fills = leadFillsFrom(contact);
          writer.save({
            full_name: fills.full_name,
            email: fills.email || lead.email,
            title: fills.title || lead.title,
            company_name: fills.company_name || lead.company_name,
          });
        }}
        pending={writer.patch.isPending}
        error={failed ? problemMessageOf(writer.patch.error, t) : undefined}
      />
    </div>
  );
}
