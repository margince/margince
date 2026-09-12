// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { ContractDraft, ContractTermsFragment } from "./contractform";

type Contract = components["schemas"]["Contract"];
type ValueBasis = ContractDraft["valueBasis"];

// The terms a create and a renewal share, turned into the fragment both send.
//
// Extracted from contractform.tsx so that file stays under the length cap, and
// it is the right seam anyway: this is pure translation from the draft to the
// wire, with no rendering in it, and both writers call exactly this.
//
// The rule throughout is omit rather than send empty. An omitted date is "not
// recorded"; an empty string is a value the server would have to reject, and
// the difference is what keeps a half-filled form from reading as a half-known
// agreement.

export function contractTermsBody(draft: ContractDraft): ContractTermsFragment {
  const body: ContractTermsFragment = {};
  if (draft.contractNumber.trim() !== "") {
    body.contract_number = draft.contractNumber.trim();
  }
  if (draft.valueMinor > 0) {
    body.value_minor = draft.valueMinor;
  }
  if (draft.arrMinor > 0) {
    body.arr_minor = draft.arrMinor;
  }
  // The currency travels whenever EITHER figure does. An agreement priced only
  // on its recurring value is a real agreement, and sending its ARR without a
  // code would be refused as a stranded figure.
  if ((draft.valueMinor > 0 || draft.arrMinor > 0) && draft.currency !== "") {
    body.currency = draft.currency;
  }
  if (draft.startsOn !== "") {
    body.starts_on = draft.startsOn;
  }
  if (draft.endsOn !== "") {
    body.ends_on = draft.endsOn;
  }
  if (draft.renewalOn !== "") {
    body.renewal_on = draft.renewalOn;
  }
  if (draft.noticePeriodDays !== "") {
    body.notice_period_days = Number(draft.noticePeriodDays);
  }
  // Emptiness, not falsiness: 0 is a real answer here and means due on
  // receipt, so a truthiness check would silently drop the one term a
  // reader is most likely to have typed deliberately.
  if (draft.paymentTermDays !== "") {
    body.payment_term_days = Number(draft.paymentTermDays);
  }
  if (draft.signedOn !== "") {
    body.signed_on = draft.signedOn;
  }
  return body;
}

// A renewal starts from its predecessor, and what it does NOT inherit is the
// point: only the two fields the wire requires are handed down.
export function renewDraftOf(predecessor: Contract): ContractDraft {
  return {
    // Title and basis are the two the successor is likeliest to keep, and the
    // wire requires both — prefilled so renewing an unchanged agreement does
    // not mean retyping its own name. Nothing else is handed down: the request
    // inherits only the counterparty, which the server derives, because a
    // renewal is a fresh negotiation and an inherited amount, notice period or
    // payment term would be a number nobody agreed to this time.
    title: predecessor.title,
    contractNumber: "",
    valueMinor: 0,
    // Not inherited either, and for exactly the reason above: a recurring
    // figure carried across a renewal is a price nobody agreed to this time.
    arrMinor: 0,
    currency: "",
    valueBasis: (predecessor.value_basis as ValueBasis) ?? "total",
    startsOn: "",
    endsOn: "",
    renewalOn: "",
    noticePeriodDays: "",
    paymentTermDays: "",
    signedOn: "",
  };
}
