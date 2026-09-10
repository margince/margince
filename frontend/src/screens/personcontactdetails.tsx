// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Details panel's email and phone rows, plus the one control that edits
// both. Split out of personrail.tsx, which still owns DetailsGrid itself —
// the two are read as one panel, this file just keeps that panel's own file
// from growing past its length ceiling.

import { Mail, Phone } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ContactLink } from "../design-system/contactlink";
import { FieldRow } from "../design-system/fieldgrid";
import { useT } from "../i18n";
import { EditContactMethodsModal } from "./personcontactedit";

type Person = components["schemas"]["Person"];

// Email and phone read as what a reader DOES with them — an address is written
// to, a number is dialled — while editing both lives behind the "Edit contact
// methods" modal (ContactMethodsEdit below), because the wire replaces the
// whole list at once (UpdatePersonRequest.emails/.phones) rather than one
// value in place.
export function EmailRow({ person }: Readonly<{ person: Person }>) {
  const t = useT();
  const primary =
    person.emails?.find((row) => row.is_primary) ?? person.emails?.[0];
  const email = primary?.email;
  return (
    <FieldRow label={t("person.rail.email")} icon={<Mail />}>
      {email ? (
        <ContactLink
          kind="email"
          value={email}
          record={{ entityType: "person", entityId: person.id }}
          readOnly={Boolean(person.archived_at)}
          className="pe-meta-link"
        />
      ) : (
        <span className="pe-rail-value-muted">{t("field.unset")}</span>
      )}
    </FieldRow>
  );
}

export function PhoneRow({ person }: Readonly<{ person: Person }>) {
  const t = useT();
  const primary =
    person.phones?.find((row) => row.is_primary) ?? person.phones?.[0];
  const phone = primary?.phone;
  return (
    <FieldRow label={t("person.rail.phone")} icon={<Phone />}>
      {phone ? (
        <ContactLink kind="phone" value={phone} className="pe-meta-link" />
      ) : (
        <span className="pe-rail-value-muted">{t("field.unset")}</span>
      )}
    </FieldRow>
  );
}

// The one write path for both rows above: a button that opens
// EditContactMethodsModal, staged from the record's own current lists. Drawn
// as DetailsGrid's own `titleAction` — the same header slot Employers below
// uses for "Add employment" — rather than paired against PhoneRow's own
// value: the modal replaces BOTH lists in one save, so the verb that opens it
// belongs to the panel rather than to either row alone.
export function ContactMethodsEdit({
  person,
  canEdit,
}: Readonly<{ person: Person; canEdit: boolean }>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  return (
    <>
      {canEdit ? (
        <Button variant="link" small onClick={() => setEditing(true)}>
          {t("person.rail.editContactMethods")}
        </Button>
      ) : null}
      <EditContactMethodsModal
        open={editing}
        onClose={() => setEditing(false)}
        person={person}
      />
    </>
  );
}
