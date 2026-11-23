// The palette's LIVE arm: the records a query finds in the workspace, beside
// the built-in commands the palette already knows.
//
// Its own file because it is its own subject — a query, a debounce, a floor, a
// refusal that must not read as an empty workspace, and the grouping that keeps
// one kind of hit from crowding out the rest. None of that is what a command
// palette IS, and all of it is what `palette.tsx` was mostly made of.

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useSettledValue } from "../design-system/debouncedsearch";
import { activityDay } from "../format/activitywhen";
import { foldForSearch } from "../format/collate";
import { useLocale, useT } from "../i18n";
import { SCREEN_ENTITY } from "../lib/entity";
import { previousOfScope, useHealthScope } from "../lib/healthscope";
import {
  problemFieldErrors,
  problemMessageOf,
  throwProblem,
} from "../lib/problem";
import type { Route } from "../lib/router";
import {
  bestIdentifierMatch,
  groupSearchHits,
  type SearchHitType,
  searchGroupHeading,
  searchHitDestination,
  searchHitGroup,
  searchHitHasCard,
  searchHitType,
} from "../lib/searchkinds";
import { useViewerZone } from "../lib/viewerzone";
import type { Command } from "./palette";
import { describeHit } from "./palettestanding";

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

// How many hits the palette offers once the reader narrows to one type.
export const PALETTE_PER_KIND = 8;

// Whether an answer for `asked` still speaks to `typed`: one extends the other,
// as typing on or backspacing does, rather than the box having changed subject.
// Case-blind, as the search itself is.
function sameSubject(asked: string, typed: string): boolean {
  const a = foldForSearch(asked);
  const b = foldForSearch(typed);
  return b.startsWith(a) || a.startsWith(b);
}

// What the live search arm has to say: the rows it found, and whether it is
// still working or gave up. The two flags are returned rather than swallowed —
// the palette used to answer a failed search with an empty array, which is the
// same shape as "no matches" and told the reader the workspace holds nothing
// when the truth was that nobody had asked it.
type SearchArm = Readonly<{
  commands: Command[];
  // How many hits of each type the answer carries; a floor where it held more.
  counts: Partial<Record<SearchHitType, number>>;
  floors: Partial<Record<SearchHitType, boolean>>;
  pending: boolean;
  failed: boolean;
  // What to tell the reader when it failed: the server's advice for a search
  // that was too broad, otherwise the shared line.
  failure: string;
  // The search answered the words now in the box and found no record.
  // False while a type is left out: nothing found there is not nothing found.
  empty: boolean;
  // The types the server could not search in time, for the answer on screen.
  timedOut: readonly string[];
}>;

// Live record hits for the palette (RS-1): one request per pause in the typing
// (useSettledValue), from two characters, and each superseded request aborted.
//
// GROUPED, a few of each kind: relevance does not compare across kinds, and a
// short list ranked across them was all mail threads for a word that also
// named a company — the company was found and never shown.
export function useSearchCommands(
  query: string,
  kind: SearchHitType | null = null,
): SearchArm {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  // Leading blanks only: a trailing one tells the server the word is finished.
  const typed = query.trimStart();
  const settled = useSettledValue(typed);
  // The typed floor hides a cleared box's hits at once (its fetch aborts a
  // pause later, when `settled` moves); the settled one never asks one letter.
  const enabled = typed.trim().length >= 2 && settled.trim().length >= 2;
  // Hits are records the reader's row scope narrows.
  const readerScope = useHealthScope();
  const result = useQuery({
    queryKey: ["palette-search", settled, kind ?? "", readerScope ?? ""],
    enabled: enabled && readerScope !== undefined,
    // The last answer stands while the next is asked: an empty list between
    // two answers reads as "no matches" for a word that has some.
    placeholderData: previousOfScope(readerScope),
    queryFn: async ({ signal }) => {
      const { data, error } = await api.GET("/search", {
        // So a keystroke the reader has typed past stops costing the server a
        // ranking that would otherwise run to its ceiling.
        signal,
        params: {
          query: {
            q: settled,
            per_type: kind ? PALETTE_PER_KIND : PALETTE_PER_TYPE,
            with_employees: true,
            typeahead: true,
            ...(kind ? { types: [kind] } : {}),
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
      return {
        q: settled,
        kind,
        hits: data.data,
        more: data.types_with_more ?? [],
        timedOut: data.types_timed_out ?? [],
      };
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
  const now = new Date();
  // Every hit with somewhere to go, asked of the one place that knows where
  // each kind lives. An activity goes to the results screen with it open in
  // its panel, the one page that already owns both drawers.
  // Kept-over hits and failures go once the words or the type change: `settled`
  // lags a burst, and another word's answer under the box misleads.
  const answer =
    enabled &&
    result.data &&
    result.data.kind === kind &&
    sameSubject(result.data.q, typed)
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
            ...describeHit(hit, secondLine(hit), { t, locale, zone, now }),
            kind: searchHitGroup(hit),
            // A message is cited the way every surface cites one; this file
            // carries the subject and the date and draws neither.
            cite: hit.email_summary
              ? {
                  subject: hit.email_summary.subject,
                  occurredAt: activityDay(
                    hit.email_summary.occurred_at,
                    hit.email_summary.occurred_precision,
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
            // An extension's hit has no destination and never reaches this
            // row, so no directory is needed to name its group.
            group: searchGroupHeading(group, t, []),
            type: "record",
            route,
            executedQuery: answer?.q,
            searchedRecord: recentRecord(hit, route),
          },
        ];
      }),
  );
  const best = bestIdentifierMatch(answer?.hits ?? []);
  const command =
    best &&
    commands.find((item) => item.id === `record:${best.type}:${best.id}`);
  if (command)
    commands.unshift({
      ...command,
      id: `best:${command.id}`,
      group: t("search.bestMatch"),
    });

  return {
    commands,
    ...hitCounts(answer?.hits ?? [], answer?.more ?? []),
    // `isFetching` rather than `isPending`: a disabled query reports pending
    // forever, and the palette opens with an empty box every time.
    // Not gated by subject: the wait is real, and an empty list beside no bar
    // would read as "no matches".
    pending: enabled && result.isFetching,
    failed,
    // Asked of the hits rather than the rows: the results page lists a hit the
    // palette has no row for. A failed refresh keeps the answer before it.
    empty: !failed && !result.isFetching && foundNoRecord(answer, typed),
    ...timedOutOf(answer),
    failure: problemMessageOf(result.error, t, t("palette.searchFailed")),
  };
}

// The answer is for the words in the box and holds no record, and no type was
// left out: a type that ran out of time may have had matches.
function foundNoRecord(
  answer:
    | Readonly<{
        q: string;
        hits: readonly unknown[];
        timedOut: readonly string[];
      }>
    | undefined,
  typed: string,
): boolean {
  return (
    answer !== undefined &&
    foldForSearch(answer.q) === foldForSearch(typed) &&
    answer.hits.length === 0 &&
    answer.timedOut.length === 0
  );
}

function timedOutOf(
  answer: Readonly<{ timedOut: readonly string[] }> | undefined,
): Pick<SearchArm, "timedOut"> {
  return { timedOut: answer?.timedOut ?? [] };
}

function hitCounts(
  hits: readonly SearchResult[],
  more: readonly string[],
): Pick<SearchArm, "counts" | "floors"> {
  const counts: Partial<Record<SearchHitType, number>> = {};
  const floors: Partial<Record<SearchHitType, boolean>> = {};
  for (const hit of hits) {
    const type = searchHitType(hit.type);
    if (!type) continue;
    counts[type] = (counts[type] ?? 0) + 1;
    floors[type] = more.includes(type);
  }
  return { counts, floors };
}

function recentRecord(
  hit: SearchResult,
  route: Route,
): Command["searchedRecord"] {
  const kind = SCREEN_ENTITY[route.screen];
  if (!kind || route.id !== hit.id) return undefined;
  return {
    kind: "record",
    entity_type: kind,
    id: hit.id,
    label: hit.title ?? hit.id,
  };
}
