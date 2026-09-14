import type { components } from "../api/schema";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// What the subject is told to quote when they ask after a request they just
// sent through their confirm link.
//
// Until the request opened a case, this page said "a contact here will apply
// your changes" and left it there. The subject had no way to chase it: no
// reference, no statement that an answer is owed, and no name for the right
// they had exercised. Art. 12(3) gives them an answer within a month, and a
// contact who cannot identify their own request cannot ask whether it came.

export type RightsCaseReceipt = components["schemas"]["RightsCaseReceipt"];

// The right each case was opened under, in the subject's own words. A literal
// map rather than a template, so an unknown kind from a newer server renders
// nothing instead of a key.
const KIND_LABEL: Record<RightsCaseReceipt["kind"], MessageKey> = {
  rectify: "confirm.receipt.rectify",
  erasure: "confirm.receipt.erasure",
};

export function RequestReceipts({
  receipts,
}: {
  receipts: RightsCaseReceipt[];
}) {
  const t = useT();
  // A submission carrying only a marketing answer opens no case, and a heading
  // over an empty list would promise an answer nobody owes.
  if (receipts.length === 0) {
    return null;
  }
  return (
    <section className="confirm-receipts">
      <h2 className="t-h3">{t("confirm.receipt.title")}</h2>
      <p className="t-body">{t("confirm.receipt.body")}</p>
      <ul className="confirm-receipts-list">
        {receipts.map((receipt) => {
          const label = KIND_LABEL[receipt.kind];
          return (
            <li key={receipt.reference} className="t-body">
              {label ? t(label) : null}
              {receipt.field ? ` · ${receipt.field}` : null}
              <code className="confirm-receipt-ref">{receipt.reference}</code>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
