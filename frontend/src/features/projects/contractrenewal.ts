// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { unwrap } from "../../lib/problem";
import type { ContractDraft } from "./contractform";
import { contractTermsBody } from "./contracttermsbody";

type Contract = components["schemas"]["Contract"];

function renewalBody(
  draft: ContractDraft,
  dealId: string,
): components["schemas"]["RenewContractRequest"] {
  return {
    title: draft.title.trim(),
    value_basis: draft.valueBasis,
    // Stated, as contractBody (contractform.tsx) states it on a create. Whether
    // the successor renews itself is a fact about the paper. An omitted field
    // would be a guess the record could not tell from an answer.
    auto_renew: false,
    // Omitted when no deal is picked, as in contractTermsBody: that is "not
    // recorded", not a value the server must refuse. Never the predecessor's
    // deal_id, which the server does not inherit either (contract_write.go). A
    // renewal is usually won by its own opportunity, so the old one would name
    // the wrong sale.
    ...(dealId !== "" ? { deal_id: dealId } : {}),
    ...contractTermsBody(draft),
  };
}

// The successor's terms, created in the transaction that supersedes the
// predecessor. The id and version come from the predecessor passed in, never
// from a closure. Otherwise a click after the modal re-rendered for another row
// would renew whichever contract the previous render held.
export async function renewContract(
  predecessor: Contract,
  draft: ContractDraft,
  dealId: string,
): Promise<string> {
  const data = unwrap(
    await api.POST("/contracts/{id}/renewal", {
      params: {
        path: { id: predecessor.id },
        ...ifMatch(requireVersion(predecessor.version)),
      },
      body: renewalBody(draft, dealId),
    }),
  );
  return data?.id ?? "";
}
