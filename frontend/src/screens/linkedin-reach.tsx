// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { EmptyState, Skeleton } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
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
      const { data, error } = await api.GET("/me/linkedin-reach", {
        params: { query: { limit: REACH_PAGE_LIMIT } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
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

// Every cell stays `white-space: nowrap` (`.li-reach-cell`): an account name
// plus two counts is wider than a phone, so DataTable's own scroller takes the
// sideways scroll and the page does not.
function ReachTable({
  accounts,
}: Readonly<{ accounts: readonly ReachAccount[] }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <DataTable
      label={t("linkedinReach.accountsLabel")}
      columns={[
        {
          key: "account",
          header: t("linkedinReach.account"),
          render: (account: ReachAccount) => (
            <a
              className="li-reach-cell "
              href={`#/companies/${account.company_id}`}
            >
              {account.display_name}
            </a>
          ),
        },
        {
          key: "connections",
          header: t("linkedinReach.connections"),
          render: (account: ReachAccount) => (
            <span className="li-reach-cell li-reach-figure">
              {/* Grouped, like the pair in the column beside it: two spellings
                  of one count in one table is the drift this closes. */}
              {formatNumber(account.connections, locale)}
            </span>
          ),
        },
        {
          key: "onFile",
          // The GAP is the finding: contacts you know there who are not
          // contacts. Rendering only the total would hide it.
          header: t("linkedinReach.onFile"),
          render: (account: ReachAccount) => (
            <span className="li-reach-cell li-reach-figure">
              {t("linkedinReach.onFileOf", {
                onFile: formatNumber(account.contacts_on_file, locale),
                total: formatNumber(account.connections, locale),
              })}
            </span>
          ),
        },
      ]}
      rows={[...accounts]}
      rowKey={(account) => account.company_id}
    />
  );
}
