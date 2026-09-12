import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

export type BillingCompany = components["schemas"]["BillingCompany"];

const ROLE_LABEL: Record<
  components["schemas"]["BillingContactRole"],
  MessageKey
> = {
  recipient: "billing.role.recipient",
  approver: "billing.role.approver",
  accounts_payable: "billing.role.accountsPayable",
};

// Whose invoices this contact handles.
//
// Beside their employers rather than inside them: where somebody WORKS and
// whose invoices they HANDLE are different facts, and an external bookkeeper
// holds the second without the first. Folding the two would put them on the
// customer's payroll on the strength of an invoice.
//
// Undefined is a withheld section and renders nothing. Empty renders nothing
// either, and that is the one place this panel differs from the company's:
// most contacts handle nobody's invoices, so an empty panel on every contact page
// in the product would be noise stating the obvious.
export function ContactBillingRoles({
  companies,
}: Readonly<{ companies?: readonly BillingCompany[] }>) {
  const t = useT();
  if (companies === undefined || companies.length === 0) {
    return null;
  }
  return (
    <Panel title={t("billing.contactTitle")}>
      <PanelBody>
        <ul className="billing-list">
          {companies.map((c) => (
            <li className="billing-row" key={c.relationship_id}>
              <div className="billing-who">
                <span className="billing-name">{c.company_name}</span>
                <Badge quiet>{t(ROLE_LABEL[c.role])}</Badge>
              </div>
            </li>
          ))}
        </ul>
      </PanelBody>
    </Panel>
  );
}
