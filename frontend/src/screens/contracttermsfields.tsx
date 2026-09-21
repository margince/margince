// The nine fields an agreement's own terms are made of — title through signed
// date — shared between RECORDING one (contractform.tsx) and RENEWING one
// (contractlifecycle.tsx). Both write the same shape of request, so this is the
// one place the fields are drawn rather than a second, driftable copy of each.
//
// Its own file because contractform.tsx sits at a frozen line ceiling and this
// component is the largest thing in it that the form's state machine does not
// touch: it takes a draft and a setter and renders controls, nothing more.
//
// The workspace's own fields are deliberately NOT here. They render beside this
// component on the record form and nowhere near the renewal, because a renewal
// is a fresh negotiation and inherits none of them.

import { Field, TextInput } from "../design-system/atoms";
import { MoneyInput } from "../design-system/moneyinput";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { ContractArrField } from "./contractarr";
import type { ContractDraft, ValueBasis } from "./contractform";

export function ContractTermsFields({
  draft,
  setDraft,
  currency,
}: Readonly<{
  draft: ContractDraft;
  setDraft: (draft: ContractDraft) => void;
  currency: string;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("contracts.form.name")} required>
        {(props) => (
          <TextInput
            {...props}
            value={draft.title}
            onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          />
        )}
      </Field>

      <Field label={t("contracts.form.number")}>
        {(props) => (
          <TextInput
            {...props}
            value={draft.contractNumber}
            onChange={(e) =>
              setDraft({ ...draft, contractNumber: e.target.value })
            }
          />
        )}
      </Field>

      <Field label={t("contracts.form.value")}>
        {(props) => (
          // MoneyInput rather than a TextInput this file scales itself, and the
          // reasons are the two defects the hand-rolled version had.
          //
          // It keeps the typed text as its OWN state, so a fractional amount is
          // not reformatted between keystrokes — typing "12.345" into a
          // three-decimal currency lost its tail when every keystroke was
          // scaled and echoed back.
          //
          // And it re-seeds when the CURRENCY changes, which this form needs
          // more than any other: the installation read that supplies the code
          // may land after the reader has already typed. Scaling on each
          // keystroke against a currency still in flight recorded the amount at
          // the two-digit fallback and then reinterpreted the same integer at
          // the real scale, with nothing on screen to say it had moved.
          <MoneyInput
            {...props}
            min={0}
            currency={currency}
            valueMinor={draft.valueMinor}
            // An agreement on record may carry no value at all — the two money
            // columns are paired and both NULL until somebody prices it — so an
            // unpriced one shows an empty field, not a nought nobody typed.
            blankWhenZero
            onChangeMinor={(valueMinor) => setDraft({ ...draft, valueMinor })}
          />
        )}
      </Field>

      <ContractArrField
        arrMinor={draft.arrMinor}
        currency={currency}
        onChangeMinor={(arrMinor) => setDraft({ ...draft, arrMinor })}
      />

      {/* The basis is asked HERE, next to the amount, because it changes what
          the amount means. An open-ended agreement has no finite total, so it
          records twelve months and says so — and a figure whose basis was
          picked on another screen is a figure nobody checked. */}
      <Field label={t("contracts.form.basis")} required>
        {(props) => (
          <Select
            {...props}
            value={draft.valueBasis}
            onChange={(value) =>
              setDraft({ ...draft, valueBasis: value as ValueBasis })
            }
            options={[
              { value: "total", label: t("contracts.basis.total") },
              { value: "annualized_12m", label: t("contracts.basis.annual") },
            ]}
          />
        )}
      </Field>

      <Field label={t("contracts.form.startsOn")}>
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.startsOn}
            onChange={(e) => setDraft({ ...draft, startsOn: e.target.value })}
          />
        )}
      </Field>

      {/* Empty means open-ended, which is a real shape rather than a missing
          answer — and it is exactly the case the annualized basis exists for. */}
      <Field
        label={t("contracts.form.endsOn")}
        hint={t("contracts.form.endsOnHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.endsOn}
            onChange={(e) => setDraft({ ...draft, endsOn: e.target.value })}
          />
        )}
      </Field>

      <Field label={t("contracts.form.renewalOn")}>
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.renewalOn}
            onChange={(e) => setDraft({ ...draft, renewalOn: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.noticeDays")}
        hint={t("contracts.form.noticeDaysHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="number"
            min={0}
            value={draft.noticePeriodDays}
            onChange={(e) =>
              setDraft({ ...draft, noticePeriodDays: e.target.value })
            }
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.paymentTerms")}
        hint={t("contracts.form.paymentTermsHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="number"
            min={0}
            value={draft.paymentTermDays}
            onChange={(e) =>
              setDraft({ ...draft, paymentTermDays: e.target.value })
            }
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.signedOn")}
        hint={t("contracts.form.signedOnHint")}
      >
        {(props) => (
          <TextInput
            {...props}
            type="date"
            value={draft.signedOn}
            onChange={(e) => setDraft({ ...draft, signedOn: e.target.value })}
          />
        )}
      </Field>
    </>
  );
}
