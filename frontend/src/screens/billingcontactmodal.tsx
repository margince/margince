import { useCallback, useEffect, useId, useState } from "react";
import { Button, Field, Modal } from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { BillingContact, BillingContactRole } from "./billingcontacts";
import type { BillingContactActions } from "./billingcontacts.queries";
import { problemMessageOf } from "./common";
import { searchByEntity } from "./relationshipcandidates";

// The three capacities, in the order an invoice moves through them: it is
// addressed to somebody, approved by somebody, then paid by somebody. Spelled
// here as well as in the panel because these are the PICKABLE ones and the
// panel's map is the readable ones — the same list today, and the server would
// refuse a fourth either way.
const ROLE_OPTIONS: { value: BillingContactRole; label: MessageKey }[] = [
  { value: "recipient", label: "billing.role.recipient" },
  { value: "approver", label: "billing.role.approver" },
  { value: "accounts_payable", label: "billing.role.accountsPayable" },
];

/**
 * Naming who handles this company's invoices, or changing the capacity one of
 * them holds.
 *
 * One modal for both. They ask the same question — which contact, in which
 * capacity — and differ only in whether the contact is already chosen: a role
 * change keeps the edge and moves it, so the contact is fixed and shown rather
 * than searched for again.
 */
export function BillingContactModal({
  open,
  onClose,
  actions,
  editing,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  actions: BillingContactActions;
  // The contact whose capacity is being changed, or undefined when naming a
  // new one.
  editing?: BillingContact;
}>) {
  const t = useT();
  const headingId = useId();
  const [contact, setContact] = useState<RecordPickerCandidate | null>(null);
  const [role, setRole] = useState<BillingContactRole>("recipient");
  const write = editing ? actions.changeRole : actions.add;
  // Pulled out because the effect below depends on THIS function rather than
  // on the mutation object, which is a new object every render: depending on
  // the object would reset the form under the reader mid-edit.
  const resetWrite = write.reset;

  // Seeded on the OPENING edge alone, so a background refetch of the finance
  // summary cannot replace a reader's unsaved pick with what the server holds.
  useEffect(() => {
    if (open) {
      setContact(null);
      setRole(editing?.role ?? "recipient");
      resetWrite();
    }
  }, [open, editing, resetWrite]);

  // Stable across renders: RecordPicker treats a new `searchTargets` identity
  // as a new search space and empties whatever it was showing.
  const searchTargets = useCallback(
    (q: string) => searchByEntity("contact", q),
    [],
  );

  async function submit() {
    if (editing) {
      await actions.changeRole.mutateAsync({ contact: editing, role });
    } else {
      if (!contact) {
        return;
      }
      await actions.add.mutateAsync({ contactId: contact.id, role });
    }
    onClose();
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {editing ? t("billing.changeTitle") : t("billing.addTitle")}
      </h2>
      <div className="form-stack">
        {editing ? (
          <div className="field">
            <span className="t-label">{t("billing.who")}</span>
            {/* Fixed, not searchable. Changing WHO would be a different edge —
                take this one off and name the other contact instead. */}
            <p>{editing.full_name}</p>
          </div>
        ) : (
          <div className="field">
            <span className="t-label">{t("billing.who")}</span>
            <RecordPicker
              label={t("billing.findContact")}
              searchTargets={searchTargets}
              selected={contact}
              onPick={setContact}
              disabled={write.isPending}
            />
          </div>
        )}
        <Field label={t("billing.role")}>
          {(control) => (
            <Select
              {...control}
              options={ROLE_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={role}
              onChange={(next) => setRole(next as BillingContactRole)}
              disabled={write.isPending}
            />
          )}
        </Field>
        <p className="t-caption">{t("billing.roleNote")}</p>
        {write.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(write.error, t)}
          </p>
        )}
        <div className="actions">
          <Button variant="ghost" onClick={onClose} disabled={write.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button
            onClick={() => {
              // The mutation already HOLDS the failure — the alert above renders
              // from its `isError`. What is swallowed here is only the promise
              // `mutateAsync` returns, which is otherwise an unhandled rejection
              // for a refusal the reader is already looking at.
              submit().catch(() => {});
            }}
            disabled={write.isPending || (!editing && !contact)}
          >
            {editing ? t("billing.saveChange") : t("billing.saveAdd")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
