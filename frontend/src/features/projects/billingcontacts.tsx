import { useState } from "react";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { billingRoleLabel } from "../../lib/recordlabels";
import { BillingContactModal } from "./billingcontactmodal";
import { useBillingContactActions } from "./billingcontacts.queries";

export type BillingContact = components["schemas"]["BillingContact"];
export type BillingContactRole = components["schemas"]["BillingContactRole"];

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
}: Readonly<{
  contacts?: readonly BillingContact[];
  companyId: string;
}>) {
  const t = useT();
  // Naming somebody, moving them or taking them off writes a relationship, not
  // the company, so the relationship grant decides whether the verbs show.
  const mayWrite = useCanWrite("relationship", "create");
  // TAKING SOMEBODY OFF IS ITS OWN VERB, and the default seat is exactly where
  // sharing the create gate showed. `rep` holds `writeNoDelete` on
  // relationship, so it passes the check above and is refused by the server on
  // Remove alone — the most common seat in the product, learning from an error
  // after pressing that the button was never for it. The fix above closed this
  // for read-only seats and left it open for the one that writes.
  const mayRemove = useCanWrite("relationship", "delete");
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
        {/* A refused removal is the one failure here a reader must not have
            to infer: the row stays and the button re-enables, which reads
            exactly like a contact who is still on the account. */}
        <ErrorLine error={actions.remove.error} />
        {contacts.length === 0 ? (
          <p className="t-caption">{t("billing.none")}</p>
        ) : (
          <ul className="billing-list">
            {contacts.map((c) => (
              <BillingContactRow
                key={c.relationship_id}
                contact={c}
                canWrite={mayWrite}
                canRemove={mayRemove}
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
  canRemove,
  busy,
  onChange,
  onRemove,
}: Readonly<{
  contact: BillingContact;
  canWrite: boolean;
  // Separate from canWrite because the server separates them: naming and
  // moving are relationship:create, taking off is relationship:delete, and the
  // default seat holds the first without the second.
  canRemove: boolean;
  busy: boolean;
  onChange: () => void;
  onRemove: () => void;
}>) {
  const t = useT();
  return (
    <li className="billing-row">
      <div className="billing-who">
        <span>{contact.full_name}</span>
        <Badge>{billingRoleLabel(contact.role, t)}</Badge>
        {canWrite && (
          <span className="billing-verbs">
            {/* Named with the row's contact, because a list of billing
                contacts draws one of these per row and "Change" alone tells a
                reader on a screen reader nothing about which one. */}
            <Button
              variant="ghost"
              onClick={onChange}
              disabled={busy}
              aria-label={t("billing.changeOne", { who: contact.full_name })}
            >
              {t("billing.change")}
            </Button>
            {canRemove && (
              <Button
                variant="ghost"
                onClick={onRemove}
                disabled={busy}
                aria-label={t("billing.removeOne", { who: contact.full_name })}
              >
                {t("billing.remove")}
              </Button>
            )}
          </span>
        )}
      </div>
      {contact.email ? (
        <a className="billing-email" href={`mailto:${contact.email}`}>
          {contact.email}
        </a>
      ) : (
        <span className="billing-email t-caption">{t("billing.noEmail")}</span>
      )}
    </li>
  );
}
