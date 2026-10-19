// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useSetAside } from "../../screens/record360";

// The account page's "Not now". Its advice rows and its moment are set aside
// by fingerprint in one per-reader store, with the Undo every record page has.
//
// The 360 is the only thing that knows which items survive, so a row goes when
// the re-read says it does. Hiding it locally on click would hide it even when
// the set-aside never reached the server.
export function useCompanySetAside(companyId: string) {
  const client = useQueryClient();
  return useSetAside<string>({
    setAside: async (fingerprint) => {
      unwrap(
        await api.POST("/companies/{id}/suggestions/dismiss", {
          params: { path: { id: companyId } },
          body: { fingerprint },
        }),
      );
    },
    restore: async (fingerprint) => {
      unwrap(
        await api.DELETE("/companies/{id}/suggestions/dismiss", {
          params: { path: { id: companyId }, query: { fingerprint } },
        }),
      );
    },
    refresh: () =>
      Promise.all([
        client.invalidateQueries({ queryKey: ["company360", companyId] }),
        // The scan serves the merged list, so it re-reads too. Otherwise a
        // dismissed model finding would stand until the next open.
        client.invalidateQueries({ queryKey: ["account-scan", companyId] }),
      ]),
    idOf: (fingerprint) => fingerprint,
  });
}

export type DeferOf = ReturnType<typeof useCompanySetAside>["deferOf"];

// The row staying put with no word reads as a click that missed, and the rep
// clicks again.
export function DismissFailed({ failure }: Readonly<{ failure: Error }>) {
  const t = useT();
  return (
    <ErrorLine>
      {`${t("co.suggest.dismissFailed")} ${problemMessageOf(failure, t)}`}
    </ErrorLine>
  );
}
