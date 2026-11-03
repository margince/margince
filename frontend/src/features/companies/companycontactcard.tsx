// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { RecordCard } from "../../design-system/recordcard";
import { useT } from "../../i18n";
import { DEAL_ROLES, dealRoleLabel } from "../../lib/recordlabels";
import { routeHash } from "../../lib/router";
import { contactRole } from "./companyrailshared";

type Contact = components["schemas"]["Company360Contact"];

/**
 * One of the account's contacts as a card: the name, their title at the
 * account, their address, and the deal seat they hold. The Way in row and the
 * Overview's Contacts card draw the same card.
 */
export function CompanyContactCard({
  contact,
  companyName,
}: Readonly<{ contact: Contact; companyName: string }>) {
  const t = useT();
  // A role the server adds before the label map learns it draws no pill
  // rather than a raw code.
  const role = contact.deal_roles
    .map((seat) => seat.role)
    .find((seat) => DEAL_ROLES.keys.includes(seat));
  const title = contactRole(contact);
  return (
    <RecordCard
      kind="contact"
      name={contact.full_name}
      identity={contact.contact_id}
      href={routeHash({ screen: "contacts", id: contact.contact_id })}
      position={
        title ? (
          <>
            {title} · {companyName}
          </>
        ) : (
          companyName
        )
      }
      email={contact.primary_email ?? undefined}
      aside={role && <Badge wrap>{dealRoleLabel(role, t)}</Badge>}
    />
  );
}
