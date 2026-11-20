// The nine fields an agreement's own terms are made of — title through signed
// date — shared between RECORDING one (contractform.tsx) and RENEWING one
// (contractlifecycle.tsx). Both write the same shape of request, so this is the
// one place the fields are drawn rather than a second, driftable copy of each.
//
// The form's state machine does not touch it: it takes a draft and a setter
// and renders controls, nothing more.
//
// The workspace's own fields are deliberately NOT here. They render beside this
// component on the record form and nowhere near the renewal, because a renewal
// is a fresh negotiation and inherits none of them.

import { dayOutOfRange } from "../../components/create";
import { Field, TextInput } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { MoneyInput } from "../../design-system/moneyinput";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import {
  type FieldLabels,
  fieldCheckSentence,
  refusalSentences,
} from "../../lib/problem";
import { ContractArrField } from "./contractarr";
import type { ContractDraft, ValueBasis } from "./contractform";

type TermsWire =
  | "title"
  | "contract_number"
  | "value_minor"
  | "value_basis"
  | "starts_on"
  | "ends_on"
  | "renewal_on"
  | "notice_period_days"
  | "payment_term_days"
  | "signed_on";

// Each term of the draft and the wire field it is sent as, which a refusal names.
export const TERMS_WIRE: readonly (readonly [
  keyof ContractDraft,
  TermsWire,
])[] = [
  ["title", "title"],
  ["contractNumber", "contract_number"],
  ["valueMinor", "value_minor"],
  ["valueBasis", "value_basis"],
  ["startsOn", "starts_on"],
  ["endsOn", "ends_on"],
  ["renewalOn", "renewal_on"],
  ["noticePeriodDays", "notice_period_days"],
  ["paymentTermDays", "payment_term_days"],
  ["signedOn", "signed_on"],
];

export function termsLabels(t: Translator): Record<TermsWire, string> {
  return {
    title: t("contracts.form.name"),
    contract_number: t("contracts.form.number"),
    value_minor: t("contracts.form.value"),
    value_basis: t("contracts.form.basis"),
    starts_on: t("contracts.form.startsOn"),
    ends_on: t("contracts.form.endsOn"),
    renewal_on: t("contracts.form.renewalOn"),
    notice_period_days: t("contracts.form.noticeDays"),
    payment_term_days: t("contracts.form.paymentTerms"),
    signed_on: t("contracts.form.signedOn"),
  } satisfies FieldLabels;
}

// A count of days the server takes: a whole number, zero or more.
function daysFault(
  text: string,
  locale: Locale,
  label: string,
): string | undefined {
  const typed = text.trim();
  if (typed === "") return undefined;
  if (!/^-?\d+$/.test(typed)) {
    return fieldCheckSentence("not_integer", locale, label);
  }
  return Number(typed) < 0
    ? fieldCheckSentence("out_of_range", locale, label, { min: "0" })
    : undefined;
}

/** What the terms refuse before sending, by wire field, in the server's words. */
export function termsFaults(
  draft: ContractDraft,
  locale: Locale,
  t: Translator,
): ReadonlyMap<string, string> {
  const labels = termsLabels(t);
  const faults = [
    ["starts_on", dayOutOfRange(draft.startsOn, locale, labels.starts_on)],
    ["ends_on", dayOutOfRange(draft.endsOn, locale, labels.ends_on)],
    ["renewal_on", dayOutOfRange(draft.renewalOn, locale, labels.renewal_on)],
    [
      "notice_period_days",
      daysFault(draft.noticePeriodDays, locale, labels.notice_period_days),
    ],
    [
      "payment_term_days",
      daysFault(draft.paymentTermDays, locale, labels.payment_term_days),
    ],
    ["signed_on", dayOutOfRange(draft.signedOn, locale, labels.signed_on)],
  ] as const;
  return new Map(
    faults.flatMap(([field, fault]) => (fault ? [[field, fault]] : [])),
  );
}

/**
 * The terms' refusals, before sending and from the server, each until its term changes.
 * The ref of the form's fields comes apart from them, as `useFieldFaults` gives it.
 */
export function useTermsFaults(
  draft: ContractDraft,
  setDraft: (draft: ContractDraft) => void,
) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults();
  const checked = termsFaults(draft, locale, t);
  const labels = termsLabels(t);
  const terms = {
    errors: (field: TermsWire) => checked.get(field) ?? faults.of(field),
    setDraft: (next: ContractDraft) => {
      for (const [key, wire] of TERMS_WIRE) {
        if (next[key] !== draft[key]) faults.edited(wire);
      }
      setDraft(next);
    },
    // True when a term is refused before sending; focus moves to it.
    refused: () => faults.show(checked),
    showRefusal: (error: unknown) =>
      faults.show(refusalSentences(error, t, labels).byField),
    // What the foot of the form says about a refusal no term stands beside.
    rest: (error: unknown) =>
      error ? refusalSentences(error, t, labels).rest : null,
    clear: () => faults.show(new Map()),
  };
  return [terms, faultsRoot] as const;
}

export function ContractTermsFields({
  draft,
  setDraft,
  currency,
  errors,
}: Readonly<{
  draft: ContractDraft;
  setDraft: (draft: ContractDraft) => void;
  currency: string;
  // The sentence that refuses a term, by its wire field (TERMS_WIRE).
  errors?: (field: TermsWire) => string | undefined;
}>) {
  const t = useT();
  return (
    <div className="form-stack">
      <Field
        label={t("contracts.form.name")}
        required
        error={errors?.("title")}
      >
        {(props) => (
          <TextInput
            {...props}
            value={draft.title}
            onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.number")}
        error={errors?.("contract_number")}
      >
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

      <Field label={t("contracts.form.value")} error={errors?.("value_minor")}>
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
      <Field
        label={t("contracts.form.basis")}
        required
        error={errors?.("value_basis")}
      >
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

      <Field label={t("contracts.form.startsOn")} error={errors?.("starts_on")}>
        {(props) => (
          <DateInput
            {...props}
            value={isoOrBlank(draft.startsOn)}
            onChange={(e) => setDraft({ ...draft, startsOn: e.target.value })}
          />
        )}
      </Field>

      {/* Empty means open-ended, which is a real shape rather than a missing
          answer — and it is exactly the case the annualized basis exists for. */}
      <Field
        label={t("contracts.form.endsOn")}
        error={errors?.("ends_on")}
        hint={t("contracts.form.endsOnHint")}
      >
        {(props) => (
          <DateInput
            {...props}
            value={isoOrBlank(draft.endsOn)}
            onChange={(e) => setDraft({ ...draft, endsOn: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.renewalOn")}
        error={errors?.("renewal_on")}
      >
        {(props) => (
          <DateInput
            {...props}
            value={isoOrBlank(draft.renewalOn)}
            onChange={(e) => setDraft({ ...draft, renewalOn: e.target.value })}
          />
        )}
      </Field>

      <Field
        label={t("contracts.form.noticeDays")}
        error={errors?.("notice_period_days")}
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
        error={errors?.("payment_term_days")}
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
        error={errors?.("signed_on")}
        hint={t("contracts.form.signedOnHint")}
      >
        {(props) => (
          <DateInput
            {...props}
            value={isoOrBlank(draft.signedOn)}
            onChange={(e) => setDraft({ ...draft, signedOn: e.target.value })}
          />
        )}
      </Field>
    </div>
  );
}
