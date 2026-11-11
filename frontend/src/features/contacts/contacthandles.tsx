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
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
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

// The primary first, then the order the writer gave. An archived one is out
// of service, so the column never offers it.
function liveHandles<T extends Email | Phone>(
  rows: readonly T[] | undefined,
): T[] {
  return (rows ?? [])
    .filter((row) => !row.archived_at)
    .sort(
      (a, b) =>
        Number(b.is_primary) - Number(a.is_primary) || a.position - b.position,
    );
}

/** Every live address of the contact, a line each with its kind. */
export function EmailHandles({
  contact,
  bought,
}: Readonly<{ contact: Contact; bought: Bought }>) {
  const t = useT();
  const rows = liveHandles(contact.emails);
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
  const rows = liveHandles(contact.phones);
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
            : t("contact.handles.more", {
                count: formatNumber(lines.length - SHOWN_FOLDED, locale),
              })}
        </Button>
      )}
    </span>
  );
}
