// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { MergeAction } from "../../../components/merge";
import type { RecordPickerCandidate } from "../../../design-system/recordpicker";
import { useT } from "../../../i18n";
import { unwrap } from "../../../lib/problem";

type Deal = components["schemas"]["Deal"];

// The "Merge into…" verb of a deal: folds this deal into another open deal of
// the same account. The server refuses deals of two companies, so the picker
// offers only this account's, and a deal on no account has nobody to merge with.
export function DealMergeAction({
  deal,
  disabledReasonId,
}: Readonly<{ deal: Deal; disabledReasonId?: string }>) {
  const t = useT();
  const companyId = deal.company_id;
  if (!companyId) {
    return null;
  }
  const searchTargets = async (q: string): Promise<RecordPickerCandidate[]> => {
    const data = unwrap(
      await api.GET("/deals", {
        params: {
          query: { q, company_id: companyId, status: "open", limit: 10 },
        },
      }),
      t,
    );
    return data.data.map((candidate) => ({
      id: candidate.id,
      name: candidate.name,
      writable: candidate.writable,
    }));
  };
  return (
    <MergeAction
      requires={{ object: "deal", action: "update" }}
      disabledReasonId={disabledReasonId}
      label={t("merge.deal")}
      sourceId={deal.id}
      sourceName={deal.name}
      searchTargets={searchTargets}
      merge={async (targetId) =>
        unwrap(
          await api.POST("/deals/{id}/merge", {
            params: {
              path: { id: deal.id },
              ...ifMatch(requireVersion(deal.version)),
            },
            body: { target_id: targetId },
          }),
          t,
        )
      }
      invalidate="deals"
      recordKey="deal"
      survivorRoute={(targetId) => ({ screen: "deals", id: targetId })}
    />
  );
}
