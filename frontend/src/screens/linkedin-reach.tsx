// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { EmptyState, Skeleton } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { RowOpen } from "../design-system/rowopen";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";
import { navigate } from "../lib/router";
import "./linkedin-reach.css";

// Which accounts a member's imported network reaches: the answer the whole
// import is for, and the half that has content on a one-member workspace.

type LinkedInReach = components["schemas"]["LinkedInReachResponse"];
type ReachAccount = LinkedInReach["accounts"][number];

// The largest page the contract's `limit` admits, asked for explicitly. This
// view has no cursor (the endpoint declares no cursor parameter and the
// response carries none), so ONE page is the whole of what a reader can reach,
// which makes where the list stops this screen's stated choice rather than the
// server's unnamed default of 50. The caveat reads `accounts_total`,
// computed over everything, so what sits past the edge is still counted out
// loud.
const REACH_PAGE_LIMIT = 200;

const REACH_KEY = ["linkedin-reach", REACH_PAGE_LIMIT] as const;

function useLinkedInReach() {
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
 * What this view cannot show, read before the figures it qualifies: where a
 * list stops, or that an empty list hides connections matching nothing yet.
 * `undefined` when there is nothing to say.
 */
function reachCaveat(
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

/** Which companies on file this member's imported network reaches, as one
 * part of the LinkedIn network panel. */
export function LinkedInReachSection() {
  const t = useT();
  const { locale } = useLocale();
  const query = useLinkedInReach();
  if (query.isPending) {
    return <Skeleton width="70%" />;
  }
  // A failed read is not an empty one: "no companies reached" over the
  // server's refusal would look exactly like a network that reaches nobody.
  if (query.isError) {
    return (
      <Callout
        kind="outcome"
        tone="danger"
        title={t("linkedinReach.readFailed")}
      >
        {problemMessageOf(query.error, t)}
      </Callout>
    );
  }
  const accounts = query.data.accounts;
  const caveat = reachCaveat(t, locale, query.data, accounts.length);
  if (accounts.length === 0) {
    // A fresh workspace that imported thousands and matched nothing should see
    // the thousands, so the unresolved count replaces the generic sentence.
    return (
      <EmptyState title={t("linkedinReach.empty")}>
        {caveat ?? t("linkedinReach.emptyBody")}
      </EmptyState>
    );
  }
  return (
    <div className="li-reach" data-testid="linkedin-reach-table">
      {/* Above the figures, not a footnote: a truncated list read as the whole
          network understates reach. */}
      <p className="t-sub">{caveat}</p>
      <ReachTable accounts={accounts} />
    </div>
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
