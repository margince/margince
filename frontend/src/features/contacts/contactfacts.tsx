// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { rosterOwnerName } from "../../components/entityref";
import { ContactLink } from "../../design-system/contactlink";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Popover } from "../../design-system/popover";
import { provenanceOf } from "../../design-system/provenanceof";
import { Fact, RecordFacts } from "../../design-system/recordfacts";
import { ProvenanceTag } from "../../design-system/trust";
import { primaryPhone } from "../../format/contacturi";
import { formatDateAbbrev } from "../../format/format";
import { linkedinUrl } from "../../format/weburl";
import { useLocale, useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { useRecordZone } from "../../lib/recordzone";
import { useViewerId } from "../../lib/session";
import { buyingRoleLabel } from "../companies/companycontacts/summary";

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

// The buying role is what the relationship edge records, never read off a
// title. A contact with a title can still have none, and then the cell is gone.
export function ContactFacts({
  contact,
  buyingRole,
}: Readonly<{ contact: Contact; buyingRole?: string | null }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const ownerName = useMemberName(contact.owner_id);
  const email = contact.primary_email;
  const phone = primaryPhone(contact.phones)?.phone;
  return (
    <RecordFacts>
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
          <ContactLink
            kind="phone"
            value={phone}
            record={{ entityType: "contact", entityId: contact.id }}
            className="pe-meta-link"
          >
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
      <Fact label={t("contact.page.owner")}>
        {rosterOwnerName(
          contact.owner_id,
          ownerName,
          t,
          t("contact.page.ownerUnassigned"),
        )}
      </Fact>
      {buyingRole && (
        <Fact label={t("contact.page.buyingRole")}>
          {buyingRoleLabel(buyingRole, t)}
        </Fact>
      )}
      <Fact label={t("list.created")}>
        {formatDateAbbrev(contact.created_at, locale, zone)}
      </Fact>
      <Fact label={t("history.field.source")}>
        <ContactSource contact={contact} />
      </Fact>
    </RecordFacts>
  );
}

/**
 * Who wrote the contact, as a provenance pill. The pill names the kind of
 * source. The popover behind it answers which one, which for a mailbox capture
 * is a message id no reader should have to scan past.
 */
export function ContactSource({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const viewerId = useViewerId();
  return (
    <Popover
      label={
        <ProvenanceTag
          provenance={provenanceOf(
            contact.captured_by,
            viewerId,
            contact.author,
            contact.source,
            contact.imported_from,
          )}
        />
      }
    >
      <p className="t-body">{contact.source || t("trust.sourceUnknown")}</p>
    </Popover>
  );
}
