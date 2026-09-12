import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

export type BillingContact = components["schemas"]["BillingContact"];
export type BillingContactRole = components["schemas"]["BillingContactRole"];

// The three capacities, in the order an invoice moves through them: it is
// addressed to somebody, approved by somebody, then paid by somebody. The
// server returns them in this order too; the map is here so a role the reader
// sees carries a word rather than a database token.
const ROLE_LABEL: Record<BillingContactRole, MessageKey> = {
  recipient: "billing.role.recipient",
  approver: "billing.role.approver",
  accounts_payable: "billing.role.accountsPayable",
};

// Who handles this account's invoices.
//
// `contacts` UNDEFINED and `contacts` EMPTY are different answers and render
// differently. Undefined is the server saying the reader lacks the grant, so
// the panel does not appear at all — a "nobody is named" message would be a
// claim about the account that this reader has no standing to make. Empty is a
// real fact and says so, because a paying customer with no recipient on file is
// a gap somebody should close.
export function BillingContactsPanel({
  contacts,
}: Readonly<{ contacts?: readonly BillingContact[] }>) {
  const t = useT();
  if (contacts === undefined) {
    return null;
  }
  return (
    <Panel title={t("billing.title")}>
      <PanelBody>
        {contacts.length === 0 ? (
          <p className="muted">{t("billing.none")}</p>
        ) : (
          <ul className="billing-list">
            {contacts.map((c) => (
              <BillingContactRow key={c.relationship_id} contact={c} />
            ))}
          </ul>
        )}
      </PanelBody>
    </Panel>
  );
}

// One contact, their capacity, and where to reach them.
//
// A missing email is shown as a stated absence rather than an empty line. It is
// the one thing about a billing contact that stops an invoice from arriving,
// and a blank row reads as "fine" to somebody scanning the list.
function BillingContactRow({ contact }: Readonly<{ contact: BillingContact }>) {
  const t = useT();
  return (
    <li className="billing-row">
      <div className="billing-who">
        <span className="billing-name">{contact.full_name}</span>
        <Badge quiet>{t(ROLE_LABEL[contact.role])}</Badge>
      </div>
      {contact.email ? (
        <a className="billing-email" href={`mailto:${contact.email}`}>
          {contact.email}
        </a>
      ) : (
        <span className="billing-email muted">{t("billing.noEmail")}</span>
      )}
    </li>
  );
}
