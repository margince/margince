import { useState } from "react";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BillingContactModal } from "./billingcontactmodal";
import { useBillingContactActions } from "./billingcontacts.queries";
import { problemMessageOf } from "./common";

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
  companyId,
  readOnly = false,
}: Readonly<{
  contacts?: readonly BillingContact[];
  companyId: string;
  // The company's own write refusal — archived, or not this caller's to
  // write. The panel does not re-derive it: the server refuses an
  // unauthorized write whatever this says, and a second implementation of the
  // gate here is the defect rather than the protection.
  readOnly?: boolean;
}>) {
  const t = useT();
  // Naming somebody, moving them, or taking them off are all writes to a
  // RELATIONSHIP — not to the company, whose own archive state arrives as
  // `readOnly` above. Both have to hold: the caller's grant plus a seat that
  // may mutate at all, and a company that still takes changes. Without the
  // grant half, a read-seat colleague was shown three enabled buttons and
  // learned they could not use them from a refusal after submitting.
  const mayWrite = useCanWrite("relationship", "create") && !readOnly;
  const actions = useBillingContactActions(
    companyId,
    t("billing.versionUnresolved"),
  );
  // Which row the modal is editing: a contact whose capacity is changing,
  // "new" to name one, or null for closed. One piece of state rather than an
  // open flag beside a selected row, so the two cannot disagree.
  const [editing, setEditing] = useState<BillingContact | "new" | null>(null);
  if (contacts === undefined) {
    return null;
  }
  return (
    <Panel
      title={t("billing.title")}
      actions={
        !mayWrite ? undefined : (
          <Button variant="ghost" onClick={() => setEditing("new")}>
            {t("billing.add")}
          </Button>
        )
      }
    >
      <PanelBody>
        {actions.remove.isError && (
          // A refused removal is the one failure here a reader must not have
          // to infer: the row stays and the button re-enables, which reads
          // exactly like a contact who is still on the account.
          <p className="t-caption" role="alert">
            {problemMessageOf(actions.remove.error, t)}
          </p>
        )}
        {contacts.length === 0 ? (
          <p className="muted">{t("billing.none")}</p>
        ) : (
          <ul className="billing-list">
            {contacts.map((c) => (
              <BillingContactRow
                key={c.relationship_id}
                contact={c}
                canWrite={mayWrite}
                busy={actions.remove.isPending}
                onChange={() => setEditing(c)}
                onRemove={() => actions.remove.mutate(c)}
              />
            ))}
          </ul>
        )}
      </PanelBody>
      <BillingContactModal
        open={editing !== null}
        onClose={() => setEditing(null)}
        actions={actions}
        editing={editing === "new" ? undefined : (editing ?? undefined)}
      />
    </Panel>
  );
}

// One contact, their capacity, and where to reach them.
//
// A missing email is shown as a stated absence rather than an empty line. It is
// the one thing about a billing contact that stops an invoice from arriving,
// and a blank row reads as "fine" to somebody scanning the list.
function BillingContactRow({
  contact,
  canWrite,
  busy,
  onChange,
  onRemove,
}: Readonly<{
  contact: BillingContact;
  canWrite: boolean;
  busy: boolean;
  onChange: () => void;
  onRemove: () => void;
}>) {
  const t = useT();
  return (
    <li className="billing-row">
      <div className="billing-who">
        <span className="billing-name">{contact.full_name}</span>
        <Badge>{t(ROLE_LABEL[contact.role])}</Badge>
        {canWrite && (
          <span className="billing-verbs">
            {/* Named with the row's contact, because a list of billing
                contacts draws one of these per row and "Change" alone tells a
                reader on a screen reader nothing about which one. */}
            <Button
              small
              variant="ghost"
              onClick={onChange}
              disabled={busy}
              aria-label={t("billing.changeOne", { who: contact.full_name })}
            >
              {t("billing.change")}
            </Button>
            <Button
              small
              variant="ghost"
              onClick={onRemove}
              disabled={busy}
              aria-label={t("billing.removeOne", { who: contact.full_name })}
            >
              {t("billing.remove")}
            </Button>
          </span>
        )}
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
