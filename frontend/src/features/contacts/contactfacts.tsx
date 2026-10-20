// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { ContactLink } from "../../design-system/contactlink";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Fact, RecordFacts } from "../../design-system/recordfacts";
import { linkedinUrl } from "../../format/weburl";
import { useT } from "../../i18n";

type Contact = components["schemas"]["Contact"];

// The fixed word claims the destination is LinkedIn, and a crawl or connector
// can write any host into `social`. So the host is checked before the anchor
// is drawn; otherwise the word stands alone.
function ProfileLink({ href }: Readonly<{ href: string }>) {
  const t = useT();
  const label = t("contact.page.linkedin");
  if (!linkedinUrl(href)) {
    return label;
  }
  return (
    <OffsiteLink href={href} className="pe-meta-link">
      {label}
    </OffsiteLink>
  );
}

// What they do and where, then the ways to reach them. Owner and buying role
// stand in the fields column and beside the name, so the line does not repeat
// them.
export function ContactFacts({
  contact,
  role,
}: Readonly<{ contact: Contact; role?: ReactNode }>): ReactNode {
  const t = useT();
  const email = contact.emails?.[0]?.email;
  const phone = contact.phones?.[0]?.phone;
  return (
    <RecordFacts>
      {role ? <Fact label={t("history.field.title")}>{role}</Fact> : null}
      {email && (
        <Fact label={t("history.field.email")}>
          <ContactLink
            kind="email"
            value={email}
            record={{ entityType: "contact", entityId: contact.id }}
            readOnly={Boolean(contact.archived_at)}
            className="pe-meta-link"
          >
            {email}
          </ContactLink>
        </Fact>
      )}
      {phone && (
        <Fact label={t("create.phone")}>
          <ContactLink kind="phone" value={phone} className="pe-meta-link">
            {phone}
          </ContactLink>
        </Fact>
      )}
      {contact.address?.city && (
        <Fact label={t("create.city")}>{contact.address.city}</Fact>
      )}
      {typeof contact.social?.linkedin === "string" && (
        <Fact label={t("contact.page.linkedin")}>
          <ProfileLink href={contact.social.linkedin} />
        </Fact>
      )}
    </RecordFacts>
  );
}
