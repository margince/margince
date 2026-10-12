// The palette's LIVE arm: the records a query finds in the workspace, beside
// the built-in commands the palette already knows.
//
// Its own file because it is its own subject — a query, a debounce, a floor, a
// refusal that must not read as an empty workspace, and the grouping that keeps
// one kind of hit from crowding out the rest. None of that is what a command
// palette IS, and all of it is what `palette.tsx` was mostly made of.

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useSettledValue } from "../design-system/debouncedsearch";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  problemFieldErrors,
  problemMessageOf,
  throwProblem,
} from "../screens/common";
import type { Command } from "./palette";
import { useRecordZone } from "./recordzone";
import {
  groupSearchHits,
  SEARCH_GROUP_KEY,
  searchHitDestination,
  searchHitHasCard,
} from "./searchkinds";

type SearchResult = components["schemas"]["SearchResult"];

// The code the server answers a search it could not rank in time under.
const TOO_BROAD = "query_too_broad";

// How long a palette search may take before the wait is worth reporting. Below
// this the answer is quicker than a keystroke and a placeholder would flash on
// every letter typed; above it, an unchanged list reads as a palette that has
// stopped listening.
export const SEARCH_PENDING_DELAY_MS = 300;

// How many hits of each kind the palette offers. Enough to pick the one meant
// out of a short list, few enough that every kind the word found still fits on
// screen; the rest are on the results page, one row away.
export const PALETTE_PER_TYPE = 3;

// Whether an answer for `asked` still speaks to `typed`: one extends the other,
// as typing on or backspacing does, rather than the box having changed subject.
// Case-blind, as the search itself is.
function sameSubject(asked: string, typed: string): boolean {
  const a = asked.toLowerCase();
  const b = typed.toLowerCase();
  return b.startsWith(a) || a.startsWith(b);
}

// What the live search arm has to say: the rows it found, and whether it is
// still working or gave up. The two flags are returned rather than swallowed —
// the palette used to answer a failed search with an empty array, which is the
// same shape as "no matches" and told the reader the workspace holds nothing
// when the truth was that nobody had asked it.
type SearchArm = Readonly<{
  commands: Command[];
  pending: boolean;
  failed: boolean;
  // What to tell the reader when it failed: the server's advice for a search
  // that was too broad, otherwise the shared line.
  failure: string;
}>;

// Live record hits for the palette (RS-1): one request per pause in the typing
// (useSettledValue), from two characters, and each superseded request aborted.
//
// GROUPED, a few of each kind: relevance does not compare across kinds, and a
// short list ranked across them was all mail threads for a word that also
// named a company — the company was found and never shown.
export function useSearchCommands(query: string): SearchArm {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const typed = query.trim();
  const settled = useSettledValue(typed);
  // The typed floor hides a cleared box's hits at once (its fetch aborts a
  // pause later, when `settled` moves); the settled one never asks one letter.
  const enabled = typed.length >= 2 && settled.length >= 2;
  const result = useQuery({
    queryKey: ["palette-search", settled],
    enabled,
    // The last answer stands while the next is asked: an empty list between
    // two answers reads as "no matches" for a word that has some.
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.GET("/search", {
        // So a keystroke the reader has typed past stops costing the server a
        // ranking that would otherwise run to its ceiling.
        signal,
        params: {
          query: {
            q: settled,
            per_type: PALETTE_PER_TYPE,
            with_employees: true,
          },
        },
      });
      if (error) {
        // Thrown rather than flattened to an empty list: react-query carries it
        // to `isError`, and the palette says the search failed instead of
        // reporting an empty workspace. The builtin commands keep working
        // beside it, which is the degradation that was wanted — losing the
        // sentence was not.
        //
        // A search refused as too broad carries its own advice (type another
        // word, narrow by kind), which is the one thing the reader can act on.
        if (
          problemFieldErrors(error).some((fault) => fault.code === TOO_BROAD)
        ) {
          throwProblem(error, t);
        }
        throw new Error(t("palette.searchFailed"));
      }
      // Paired with the words it answered: a kept-over answer is routed by
      // its own query, not by the one still being asked.
      return { q: settled, hits: data.data };
    },
  });
  // The second line says which one of a kind this is; the group heading
  // already says what kind. A project's is its key and its account, which is
  // how two projects called "Rollout" are told apart, and the server sends it.
  const secondLine = (hit: SearchResult): string | undefined => {
    // A partner is a property of a company rather than a kind of its own.
    if (hit.type === "company" && hit.is_partner === true) {
      return t("search.partner.badge");
    }
    // A contact found through its employer, which is why it is listed at all.
    if (hit.works_at) {
      return t("search.contact.worksAt", {
        company: hit.works_at.company_name,
      });
    }
    return hit.snippet ?? undefined;
  };
  // Every hit with somewhere to go, asked of the one place that knows where
  // each kind lives. An activity that is not a message has no page and drops
  // out by answering null; an EMAIL goes to the results screen with that
  // message open, the one page that already owns its drawer.
  // Kept-over hits and failures go once the words change subject: `settled`
  // lags a burst, and another word's answer under the box misleads.
  const answer =
    enabled && result.data && sameSubject(result.data.q, typed)
      ? result.data
      : undefined;
  const failed = enabled && result.isError && sameSubject(settled, typed);
  const commands = groupSearchHits(answer?.hits ?? []).flatMap(
    ({ group, hits }) =>
      hits.flatMap((hit): Command[] => {
        const route = answer ? searchHitDestination(hit, answer.q) : null;
        if (!route) {
          return [];
        }
        return [
          {
            id: `record:${hit.type}:${hit.id}`,
            label: hit.title ?? hit.id,
            subtitle: secondLine(hit),
            // A message is cited the way every surface cites one; this file
            // carries the subject and the date and draws neither.
            cite: hit.email_summary
              ? {
                  subject: hit.email_summary.subject,
                  occurredAt: formatDate(
                    hit.email_summary.occurred_at,
                    locale,
                    zone,
                  ),
                }
              : undefined,
            mark: searchHitHasCard(hit.type)
              ? {
                  identity: hit.id,
                  name: hit.title ?? hit.id,
                  logo: hit.logo_url,
                }
              : undefined,
            group: t(SEARCH_GROUP_KEY[group]),
            type: "record",
            route,
          },
        ];
      }),
  );
  return {
    commands,
    // `isFetching` rather than `isPending`: a disabled query reports pending
    // forever, and the palette opens with an empty box every time.
    // Not gated by subject: the wait is real, and an empty list beside no bar
    // would read as "no matches".
    pending: enabled && result.isFetching,
    failed,
    failure: problemMessageOf(result.error, t, t("palette.searchFailed")),
  };
}
