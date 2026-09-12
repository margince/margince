import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { type ContractDraft, contractBody } from "./contractform";

// The two writes the contract form makes. Extracted from contractform.tsx so
// that file stays under the length cap; both are called from one place and
// neither is reused elsewhere.

type Contract = components["schemas"]["Contract"];

export async function createContract(
  companyId: string,
  draft: ContractDraft,
): Promise<string> {
  const { data, error } = await api.POST("/contracts", {
    body: contractBody(companyId, draft),
  });
  if (error) {
    throwProblem(error);
  }
  return data?.id ?? "";
}

// A correction sends nulls for the fields a human cleared: once somebody has
// removed a value, "I typed this by mistake" and "we never agreed one" are the
// same answer, and leaving the old value in place would keep asserting the
// mistake.
export async function patchContract(
  contract: Contract,
  draft: ContractDraft,
): Promise<string> {
  const { error } = await api.PATCH("/contracts/{id}", {
    params: { path: { id: contract.id } },
    body: {
      title: draft.title.trim(),
      contract_number: draft.contractNumber.trim() || null,
      value_minor: draft.valueMinor > 0 ? draft.valueMinor : null,
      // Same pairing as a create, and the same refusal to complete it with a
      // guess: an amount whose currency the form does not hold goes out as the
      // half it is, for the server to refuse in the open.
      currency:
        draft.valueMinor > 0 && draft.currency !== "" ? draft.currency : null,
      value_basis: draft.valueBasis,
      starts_on: draft.startsOn || null,
      ends_on: draft.endsOn || null,
      renewal_on: draft.renewalOn || null,
      notice_period_days: draft.noticePeriodDays
        ? Number(draft.noticePeriodDays)
        : null,
      // Emptiness, not falsiness: 0 means due on receipt and is the answer a
      // reader is most likely to have typed deliberately. A truthiness check
      // here would send null and clear the very term they just set.
      payment_term_days:
        draft.paymentTermDays !== "" ? Number(draft.paymentTermDays) : null,
      signed_on: draft.signedOn || null,
    },
  });
  if (error) {
    throwProblem(error);
  }
  return contract.id;
}
