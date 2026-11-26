// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { DataTable } from "../../design-system/datatable";
import { RowOpen } from "../../design-system/rowopen";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";

// Which accounts a member's imported network reaches: the answer the whole
// import is for, and the half that has content on a one-member workspace.

export type LinkedInReach = components["schemas"]["LinkedInReachResponse"];
type ReachAccount = LinkedInReach["accounts"][number];

// The largest page the contract's `limit` admits, so most networks are one
// read. A longer one continues from the cursor of the page before.
const REACH_PAGE_LIMIT = 200;

const REACH_KEY = ["linkedin-reach", REACH_PAGE_LIMIT] as const;

/** Every page read so far as one answer, its totals from the newest read. */
function mergeReachPages(
  pages: readonly LinkedInReach[],
): LinkedInReach | undefined {
  const newest = pages.at(-1);
  if (newest === undefined) {
    return undefined;
  }
  return { ...newest, accounts: pages.flatMap((page) => page.accounts) };
}

export function useLinkedInReach() {
  return useInfiniteQuery({
    queryKey: REACH_KEY,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }): Promise<LinkedInReach> =>
      unwrap(
        await api.GET("/me/linkedin-reach", {
          params: { query: { limit: REACH_PAGE_LIMIT, cursor: pageParam } },
        }),
      ),
    // The last page carries no cursor, and neither does a server older than paging.
    getNextPageParam: (last) => last.next_cursor,
    select: (data) => mergeReachPages(data.pages),
  });
}

/**
 * What this view cannot show, read before the figures it qualifies: where a
 * list stops, or that an empty list hides connections matching nothing yet.
 * `undefined` when there is nothing to say.
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
