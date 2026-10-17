// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { DataTable } from "../../design-system/datatable";
import { RowOpen } from "../../design-system/rowopen";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";

// Which accounts a member's imported network reaches (ADR-0078 §2.1b) — the
// answer the whole import is for.
//
// It is the half that works on a one-contact workspace: asking which COLLEAGUE
// knows an account has no content when there is one member, while asking
// whether your own network reaches it has content immediately.
//
// What the list cannot show is stated above the figures it qualifies.

export type LinkedInReach = components["schemas"]["LinkedInReachResponse"];
type ReachAccount = LinkedInReach["accounts"][number];

// The largest page the contract's `limit` admits, asked for explicitly. This
// view has no cursor — the endpoint declares no cursor parameter and the
// response carries none — so ONE page is the whole of what a reader can reach,
// which makes where the list stops this screen's stated choice rather than the
// server's unnamed default of 50. The row's caveat reads `accounts_total`,
// computed over everything, so what sits past the edge is still counted out
// loud.
const REACH_PAGE_LIMIT = 200;

const REACH_KEY = ["linkedin-reach", REACH_PAGE_LIMIT] as const;

export function useLinkedInReach() {
  return useQuery({
    queryKey: REACH_KEY,
    queryFn: async (): Promise<LinkedInReach> => {
      return unwrap(
        await api.GET("/me/linkedin-reach", {
          params: { query: { limit: REACH_PAGE_LIMIT } },
        }),
      );
    },
  });
}

/**
 * The card's caveat: what this view cannot show, in the terms the state calls
 * for.
 *
 * Two sentences, never both — a reader looking at a list needs to know where it
 * stops, and a reader looking at an empty one needs to know that the
 * connections exist and simply match nothing on file yet. Neither is a
 * footnote: they qualify the figures, so they read before them.
 */
export function reachCaveat(
  t: ReturnType<typeof useT>,
  locale: Locale,
  reach: LinkedInReach | undefined,
  shown: number,
): string | undefined {
  const unresolved = reach?.unresolved_connections ?? 0;
  if (shown === 0) {
    return unresolved > 0
      ? t("linkedinReach.allUnresolved", {
          unresolved: formatNumber(unresolved, locale),
        })
      : undefined;
  }
  return t("linkedinReach.footnote", {
    shown: formatNumber(shown, locale),
    total: formatNumber(reach?.accounts_total ?? shown, locale),
    unresolved: formatNumber(unresolved, locale),
  });
}

/** Which accounts this member's imported network reaches, caveat first. */
export function NetworkReach({ reach }: Readonly<{ reach: LinkedInReach }>) {
  const t = useT();
  const { locale } = useLocale();
  const caveat = reachCaveat(t, locale, reach, reach.accounts.length);
  return (
    <>
      {caveat && <p className="t-caption">{caveat}</p>}
      {reach.accounts.length > 0 && <ReachTable accounts={reach.accounts} />}
    </>
  );
}

function ReachTable({
  accounts,
}: Readonly<{ accounts: readonly ReachAccount[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const open = (account: ReachAccount) =>
    navigate({ screen: "companies", id: account.company_id });
  const onFile = (account: ReachAccount) =>
    t("linkedinReach.onFileOf", {
      onFile: formatNumber(account.contacts_on_file, locale),
      total: formatNumber(account.connections, locale),
    });
  return (
    <DataTable
      fold
      label={t("linkedinReach.accountsLabel")}
      rows={[...accounts]}
      rowKey={(account) => account.company_id}
      onRowClick={open}
      columns={[
        {
          key: "account",
          header: t("linkedinReach.account"),
          grow: true,
          render: (account: ReachAccount) => (
            <button
              type="button"
              className="cell-link"
              onClick={() => open(account)}
            >
              {account.display_name}
            </button>
          ),
        },
        {
          key: "connections",
          header: t("linkedinReach.connections"),
          align: "end",
          render: (account: ReachAccount) =>
            formatNumber(account.connections, locale),
          folded: (account: ReachAccount) =>
            plural("linkedinReach.connectionsCount", account.connections, {
              count: formatNumber(account.connections, locale),
            }),
        },
        {
          // The gap is the finding: contacts you know there who are not
          // contacts on file.
          key: "onFile",
          header: t("linkedinReach.onFile"),
          align: "end",
          render: onFile,
          folded: (account: ReachAccount) =>
            t("linkedinReach.onFileFolded", { figure: onFile(account) }),
        },
        {
          key: "open",
          header: t("table.actions"),
          headerHidden: true,
          align: "end",
          fold: "end",
          render: (account: ReachAccount) => (
            <RowOpen
              label={t("linkedinReach.openNamed", {
                name: account.display_name,
              })}
              onOpen={() => open(account)}
            />
          ),
        },
      ]}
    />
  );
}
