// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import {
  groupSearchHits,
  SEARCH_FILTER_KEY,
  SEARCH_GROUP_KEY,
  SEARCH_HIT_ORDER,
  type SearchHitType,
  searchGroupType,
  searchHitHasCard,
} from "../../app/searchkinds";
import { Button, EmptyState, SearchField } from "../../design-system/atoms";
import { FilterPills } from "../../design-system/filterpills";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../../design-system/querystates";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate, useRoute } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { SearchHit } from "./searchhit";
import { NearMatches } from "./searchnearmatches";
import { type OpenHit, useSearchPanels } from "./searchpanels";
import "./search.css";

type SearchResult = components["schemas"]["SearchResult"];

// RS-1/RS-2: the cross-object search results screen. Hits are grouped by what
// they are so a caller scanning "acme" sees the company, its contacts, its deals
// and the mail about it as separate sections rather than one ranked list.
//
// Unnarrowed, the page asks for a few of EACH type rather than the best fifty
// overall: relevance does not compare across types, and a thread that names an
// account in every paragraph outranks the account, so fifty ranked hits could
// be fifty emails with the company itself nowhere on the page. Narrowed to one
// type, it is that type's ranked list, paged.
//
// The order and the headings come from app/searchkinds.ts, which the ⌘K palette
// reads too. They were a pair of literals here until a type the server returned
// went missing from both — project hits came back ranked and were dropped on the
// floor, because a list that has to be edited by hand once per new type is a
// list somebody eventually does not edit.

// How many hits of each type the unnarrowed page shows before "Show all".
export const RESULTS_PER_TYPE = 5;

// Both page shapes ask for the contacts working at a matched company, so a
// contact the grouped page lists is still there once "Show all" narrows to it.
const WITH_EMPLOYEES = true;

// Which type the reader has narrowed to. `all` is the absence of a narrowing
// and is spelled by an ABSENT parameter, so one view has exactly one address.
export const SEARCH_TYPE_PARAM = "type";
const ALL_TYPES = "all";
type TypeFilter = SearchHitType | typeof ALL_TYPES;

function typeFilterFrom(value: string | undefined): TypeFilter {
  return SEARCH_HIT_ORDER.find((kind) => kind === value) ?? ALL_TYPES;
}

// The routed screen. The address's third segment picks the activity panel over
// the email one, and only this screen reads it.
export function SearchRoute(
  props: Readonly<{ q: string; openActivityId?: string }>,
) {
  return <SearchScreen {...props} openAs={useRoute().id3} />;
}

export function SearchScreen({
  q,
  openActivityId,
  openAs,
}: Readonly<{ q: string; openActivityId?: string; openAs?: string }>) {
  const t = useT();
  const [draft, setDraft] = useState(q);
  const { onOpen, drawers } = useSearchPanels(openActivityId, openAs);
  const [params, setParams] = useUrlParams();
  const filter = typeFilterFrom(params.get(SEARCH_TYPE_PARAM));

  const narrowTo = (next: TypeFilter) => {
    const dials = new Map(params);
    if (next === ALL_TYPES) {
      dials.delete(SEARCH_TYPE_PARAM);
    } else {
      dials.set(SEARCH_TYPE_PARAM, next);
    }
    setParams(dials);
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const next = draft.trim();
    if (next) {
      navigate({ screen: "search", id: encodeURIComponent(next) });
    }
  };

  return (
    <div className="wrap">
      <form onSubmit={submit} className="search-bar">
        <SearchField
          value={draft}
          placeholder={t("search.placeholder")}
          aria-label={t("search.placeholder")}
          onChange={(event) => setDraft(event.target.value)}
        />
      </form>
      {/* No term, no read — and therefore no gate. A query held back by `enabled`
          reports `pending` in react-query v5, so handing it to QueryGate drew
          three loading bars under this box forever, with nothing in flight: a
          bare `#/search` is reachable from the address bar, and the page it
          showed said "working on it" about a request that had not been made.
          The screen asks for a term instead. */}
      {q.trim() === "" ? (
        <EmptyState>{t("search.prompt")}</EmptyState>
      ) : (
        <>
          {/* Drawn above the results and not inside them, because it survives
              an empty answer: a reader who narrowed to Deals and found none
              needs the control that got them there in order to get back.

              The narrowing is a SERVER dial: a pill sends `types` rather than
              hiding rows already drawn, so a narrowed search is a different
              answer and not a smaller view of the same one. That is also why
              no pill carries a count — the endpoint knows no per-type total,
              and a figure derived from what happens to be on screen would be
              a number the reader could act on and could not trust. */}
          <div className="search-filter">
            <FilterPills
              label={t("search.filter.label")}
              value={filter}
              onChange={narrowTo}
              pills={[
                {
                  value: ALL_TYPES as TypeFilter,
                  label: t("search.filter.all"),
                },
                ...SEARCH_HIT_ORDER.map((kind) => ({
                  value: kind as TypeFilter,
                  label: t(SEARCH_FILTER_KEY[kind]),
                })),
              ]}
            />
          </div>
          {filter === ALL_TYPES ? (
            <GroupedResults q={q} onOpen={onOpen} onNarrow={narrowTo} />
          ) : (
            <NarrowedResults q={q} type={filter} onOpen={onOpen} />
          )}
        </>
      )}
      {drawers}
    </div>
  );
}

// A few of every type that matched, each type's best first, with a way into
// the rest of any type the server says holds more.
function GroupedResults({
  q,
  onOpen,
  onNarrow,
}: Readonly<{
  q: string;
  onOpen: OpenHit;
  onNarrow: (type: SearchHitType) => void;
}>) {
  const t = useT();
  const query = useQuery({
    queryKey: ["search", q, ALL_TYPES],
    queryFn: async () => {
      return unwrap(
        await api.GET("/search", {
          params: {
            query: {
              q,
              per_type: RESULTS_PER_TYPE,
              with_employees: WITH_EMPLOYEES,
            },
          },
        }),
      );
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("search.pending")}>
      {(data) =>
        data.data.length === 0 ? (
          <>
            <EmptyState>{t("search.empty", { q })}</EmptyState>
            <NearMatches matches={data.near_matches} />
          </>
        ) : (
          <SearchGroups
            results={data.data}
            onOpen={onOpen}
            more={{ types: data.types_with_more ?? [], onNarrow }}
          />
        )
      }
    </QueryGate>
  );
}

// One type's ranked list, paged by the server's keyset cursor.
function NarrowedResults({
  q,
  type,
  onOpen,
}: Readonly<{
  q: string;
  type: SearchHitType;
  onOpen: OpenHit;
}>) {
  const t = useT();
  const query = useInfiniteQuery({
    queryKey: ["search", q, type],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/search", {
          params: {
            query: {
              q,
              types: [type],
              limit: 50,
              with_employees: WITH_EMPLOYEES,
              cursor: pageParam ?? undefined,
            },
          },
        }),
      );
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const hits = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <QueryStates query={query} pendingLabel={t("search.pending")}>
      {hits.length === 0 ? (
        <>
          <EmptyState>{t("search.empty", { q })}</EmptyState>
          <NearMatches matches={query.data?.pages[0]?.near_matches} />
        </>
      ) : (
        <>
          <SearchGroups results={hits} onOpen={onOpen} />
          <LoadMoreButton query={query} />
        </>
      )}
    </QueryStates>
  );
}

function SearchGroups({
  results,
  onOpen,
  more,
}: Readonly<{
  results: SearchResult[];
  onOpen: OpenHit;
  // Which types hold more than is drawn, and how to show them. Absent on a
  // narrowed page, where the rest arrives by loading more instead.
  more?: Readonly<{
    types: readonly SearchHitType[];
    onNarrow: (type: SearchHitType) => void;
  }>;
}>) {
  const t = useT();
  const groups = groupSearchHits(results);
  return (
    <div className="search-groups arrive-stack">
      {groups.map(({ group, hits }, index) => {
        const heading = t(SEARCH_GROUP_KEY[group]);
        const type = searchGroupType(group);
        // Emails and activities narrow to one list and the server counts them
        // as one type, so only the first of their groups offers the rest.
        const offersMore =
          more?.types.includes(type) &&
          groups.findIndex((other) => searchGroupType(other.group) === type) ===
            index;
        return (
          <Panel
            key={group}
            title={heading}
            titleAction={
              offersMore &&
              more && (
                <Button
                  aria-label={t("search.group.showAllNamed", {
                    group: t(SEARCH_FILTER_KEY[type]),
                  })}
                  onClick={() => more.onNarrow(type)}
                >
                  {t("list.showAll")}
                </Button>
              )
            }
          >
            <PanelBody>
              <ul
                className={
                  searchHitHasCard(type) ? "record-card-list" : "search-hits"
                }
              >
                {hits.map((hit) => (
                  <SearchHit
                    key={`${hit.type}:${hit.id}`}
                    hit={hit}
                    onOpen={onOpen}
                  />
                ))}
              </ul>
            </PanelBody>
          </Panel>
        );
      })}
    </div>
  );
}
