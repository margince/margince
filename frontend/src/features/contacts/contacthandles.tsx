// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ContactLink, CopyEmail } from "../../design-system/contactlink";
import {
  EMAIL_TYPE_LABEL,
  PHONE_TYPE_LABEL,
} from "../../format/contactchannels";
import { primaryPhone } from "../../format/contacturi";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { BoughtMark, type boughtFields } from "./boughtmarks";
import "./contacthandles.css";

type Contact = components["schemas"]["Contact"];
type Email = NonNullable<Contact["emails"]>[number];
type Phone = NonNullable<Contact["phones"]>[number];
type Bought = ReturnType<typeof boughtFields>;

// Past three, the column keeps two and folds the rest behind one link.
const FOLD_AFTER = 3;
const SHOWN_FOLDED = 2;

// The one the server reaches them at leads, the rest keep the order they are
// sent in. An archived one is out of service, so the column never offers it.
function liveHandles<T extends Email | Phone>(
  rows: readonly T[] | undefined,
  isLead: (row: T) => boolean,
): T[] {
  const live = (rows ?? []).filter((row) => !row.archived_at);
  return [...live.filter(isLead), ...live.filter((row) => !isLead(row))];
}

/** Every live address of the contact, a line each with its kind. */
export function EmailHandles({
  contact,
  bought,
}: Readonly<{ contact: Contact; bought: Bought }>) {
  const t = useT();
  const rows = liveHandles(
    contact.emails,
    (row) => row.email === contact.primary_email,
  );
  const record = { entityType: "contact", entityId: contact.id } as const;
  return (
    <HandleStack
      lines={rows.map((row) => (
        <span key={row.id} className="fieldgrid-handle">
          <span className="contactlink-copyable">
            <ContactLink kind="email" value={row.email} record={record}>
              {row.email}
            </ContactLink>
            <CopyEmail address={row.email} />
          </span>
          <span className="t-caption">
            {t(labelFor(EMAIL_TYPE_LABEL, row.email_type ?? "work"))}
          </span>
          <BoughtMark
            bought={bought.get(`email:${row.id}`)}
            subject={row.email}
          />
        </span>
      ))}
    />
  );
}

/** Every live number of the contact, a line each with its kind. */
export function PhoneHandles({
  contact,
  bought,
}: Readonly<{ contact: Contact; bought: Bought }>) {
  const t = useT();
  const lead = primaryPhone(contact.phones);
  const rows = liveHandles(contact.phones, (row) => row.id === lead?.id);
  const record = { entityType: "contact", entityId: contact.id } as const;
  return (
    <HandleStack
      lines={rows.map((row) => (
        <span key={row.id} className="fieldgrid-handle">
          <ContactLink kind="phone" value={row.phone} record={record} />
          <span className="t-caption">
            {t(labelFor(PHONE_TYPE_LABEL, row.phone_type ?? "work"))}
          </span>
          <BoughtMark
            bought={bought.get(`phone:${row.id}`)}
            subject={row.phone}
          />
        </span>
      ))}
    />
  );
}

function HandleStack({ lines }: Readonly<{ lines: readonly ReactNode[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const folds = lines.length > FOLD_AFTER;
  const shown = folds && !open ? lines.slice(0, SHOWN_FOLDED) : lines;
  return (
    <span className="contacthandles">
      {shown}
      {folds && (
        <Button
          variant="link"
          className="contacthandles-more"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          {open
            ? t("contact.handles.fewer")
            : plural("contact.handles.more", lines.length - SHOWN_FOLDED, {
                count: formatNumber(lines.length - SHOWN_FOLDED, locale),
              })}
        </Button>
      )}
    </span>
  );
}
