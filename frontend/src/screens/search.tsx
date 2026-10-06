// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { ENTITY } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { navigate, routeHash } from "../app/router";
import {
  groupSearchHits,
  SEARCH_FILTER_KEY,
  SEARCH_GROUP_KEY,
  SEARCH_HIT_ORDER,
  type SearchHitType,
  type SearchRecordCardType,
  searchGroupType,
  searchHitHasCard,
  searchHitRoute,
} from "../app/searchkinds";
import { useUrlParams } from "../app/urlstate";
import { Badge, Button, EmptyState, SearchField } from "../design-system/atoms";
import { EmailEntry } from "../design-system/emailentry";
import { FilterPills } from "../design-system/filterpills";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordCard } from "../design-system/recordcard";
import { formatDateTime, formatNumber } from "../format/format";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import { LoadMoreButton, QueryGate, QueryStates, throwProblem } from "./common";
import { companyTabRoute } from "./companytab";
import { useOpenEmail } from "./openemail";
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

export function SearchScreen({
  q,
  openActivityId,
}: Readonly<{ q: string; openActivityId?: string }>) {
  const t = useT();
  const [draft, setDraft] = useState(q);
  const [openEmail, setOpenEmail] = useOpenEmail(openActivityId);
  const zone = useRecordZone();
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
            <GroupedResults
              q={q}
              onOpenEmail={setOpenEmail}
              onNarrow={narrowTo}
            />
          ) : (
            <NarrowedResults q={q} type={filter} onOpenEmail={setOpenEmail} />
          )}
        </>
      )}
      {/* One drawer over the whole results page, at page level rather than
          inside a group: two mounted dialogs would be two `aria-modal`
          elements, and the results stay legible behind the one that is open. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={zone}
        onClose={() => setOpenEmail(null)}
      />
    </div>
  );
}

// A few of every type that matched, each type's best first, with a way into
// the rest of any type the server says holds more.
function GroupedResults({
  q,
  onOpenEmail,
  onNarrow,
}: Readonly<{
  q: string;
  onOpenEmail: (activityId: string) => void;
  onNarrow: (type: SearchHitType) => void;
}>) {
  const t = useT();
  const query = useQuery({
    queryKey: ["search", q, ALL_TYPES],
    queryFn: async () => {
      const { data, error } = await api.GET("/search", {
        params: {
          query: {
            q,
            per_type: RESULTS_PER_TYPE,
            with_employees: WITH_EMPLOYEES,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("search.pending")}>
      {(data) =>
        data.data.length === 0 ? (
          <EmptyState>{t("search.empty", { q })}</EmptyState>
        ) : (
          <SearchGroups
            results={data.data}
            onOpenEmail={onOpenEmail}
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
  onOpenEmail,
}: Readonly<{
  q: string;
  type: SearchHitType;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const query = useInfiniteQuery({
    queryKey: ["search", q, type],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/search", {
        params: {
          query: {
            q,
            types: [type],
            limit: 50,
            with_employees: WITH_EMPLOYEES,
            cursor: pageParam ?? undefined,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
  const hits = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <QueryStates query={query} pendingLabel={t("search.pending")}>
      {hits.length === 0 ? (
        <EmptyState>{t("search.empty", { q })}</EmptyState>
      ) : (
        <>
          <SearchGroups results={hits} onOpenEmail={onOpenEmail} />
          <LoadMoreButton query={query} />
        </>
      )}
    </QueryStates>
  );
}

function SearchGroups({
  results,
  onOpenEmail,
  more,
}: Readonly<{
  results: SearchResult[];
  onOpenEmail: (activityId: string) => void;
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
                    onOpenEmail={onOpenEmail}
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

function SearchHit({
  hit,
  onOpenEmail,
}: Readonly<{
  hit: SearchResult;
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  // An email hit IS the canonical row — the same one the timeline draws, from
  // the same server projection. It replaces the generic title-and-snippet
  // rather than sitting beside it: the snippet was a raw 200-character slice
  // of the body, and drawing both would put two readings of one message on one
  // line. Every other hit type, activity or not, keeps what it had.
  if (hit.email_summary) {
    const summary = hit.email_summary;
    return (
      <li className="search-hit search-hit-email">
        <EmailEntry
          summary={summary}
          timestamp={formatDateTime(summary.occurred_at, locale, zone)}
          onOpen={() => onOpenEmail(summary.activity_id)}
        />
      </li>
    );
  }
  // A contact or a company is a record LISTED here, so it wears the card the
  // record rails list one in; every other kind keeps its title line.
  const type = hit.type;
  if (searchHitHasCard(type)) {
    return <RecordHit hit={hit} type={type} />;
  }
  // Where this kind goes, asked of the one place that knows. A tag is not a
  // record and a catalog row has no page of its own, and each used to be a
  // branch spelled here as well as in the palette; an activity answers null
  // because it is a link rather than a thing links hang off, and it renders as
  // plain text.
  const isTag = hit.type === "tag";
  const route = searchHitRoute(type, hit.id);
  return (
    <li className="search-hit">
      <div className="search-hit-title">
        {route ? (
          // The search API already returns the hit's display name as
          // `title` — routing through EntityRef here would re-fetch the
          // same record per hit (an N+1 GET per result) just to re-derive
          // a name we already have.
          <button
            type="button"
            className="entity-link"
            onClick={() => navigate(route)}
          >
            {hit.title ?? hit.id}
          </button>
        ) : (
          <span>{hit.title ?? hit.id}</span>
        )}
        {hitMarks(hit, t)}
      </div>
      {/* `hit.score` is deliberately not drawn. The contract bounds it to
          nothing (schema: "Relevance score"), so the retriever's raw figure
          reached the page as "relevance 280%" — a percentage of nothing, which
          a reader can neither act on nor disbelieve. It still does its job in
          the ordering the results arrive in. */}
      {/* What the word is on, so a reader can tell a live tag from one nobody
          used without opening it. Absent rather than zero when the server sent
          no number: a count it could not take is not a count of none. */}
      {isTag && hit.carried_by != null && (
        <p>
          {plural("search.tag.carriedBy", hit.carried_by, {
            count: formatNumber(hit.carried_by, locale),
          })}
        </p>
      )}
      {/* Quoted only when the excerpt is PROSE the record actually contains —
          an activity's body slice, which is a passage lifted out of a message
          and reads as one. Every other branch's excerpt is a structured
          identifier the record carries in a field: a project's key and company,
          a product's sku. Quoting “KAR-9910” claims somebody wrote that
          sentence, and the marks are what a reader would have to type to search
          for it. */}
      {hit.snippet && (
        <p>{hit.type === "activity" ? `“${hit.snippet}”` : hit.snippet}</p>
      )}
    </li>
  );
}

// A contact or a company, drawn from what the hit carries — the name, the
// logo, the employer — and never from a read per hit.
function RecordHit({
  hit,
  type,
}: Readonly<{ hit: SearchResult; type: SearchRecordCardType }>) {
  const t = useT();
  const marks = hitMarks(hit, t);
  // A contact found through its employer says so, or a reader looking for
  // Acme meets a name with no reason it is on the page.
  const reason = hit.works_at
    ? t("search.contact.worksAt", { company: hit.works_at.company_name })
    : hit.snippet;
  // The marks are facts about the record, so they share its facts row, which
  // spans the card and wraps; in the aside's track they squeezed the name.
  const facts = (reason || marks.length > 0) && (
    <span className="search-hit-marks">
      {reason && <span>{reason}</span>}
      {marks}
    </span>
  );
  return (
    <li className="search-hit">
      <RecordCard
        kind={type}
        name={hit.title ?? hit.id}
        identity={hit.id}
        href={routeHash(ENTITY[type].route(hit.id))}
        logo={hit.logo_url}
        position={facts}
      />
    </li>
  );
}

// What a hit carries beyond its name: a tier the reader would not assume, and
// whether a company is a partner.
function hitMarks(hit: SearchResult, t: Translator): ReactNode[] {
  const marks: ReactNode[] = [];
  // Nearly every stored record is `authoritative` (external and unverified are
  // reserved for connector rows), and a mark that never varies marks nothing.
  // Neither badge names the SYSTEM: the hit carries no provider field.
  if (hit.trust_tier === "external") {
    marks.push(
      <Badge key="tier" tone="accent">
        {t("search.tier.mirrored")}
      </Badge>,
    );
  }
  // Rare by the same contract, and a tier the record CARRIES while the page
  // draws nothing reads as a record with nothing to declare.
  if (hit.trust_tier === "unverified") {
    marks.push(
      <Badge key="tier" tone="warning">
        {t("search.tier.unverified")}
      </Badge>,
    );
  }
  // `true` alone: null is a marker nobody took, not a company found plain. The
  // route rides with the badge, or the Partners screen is reachable only by
  // knowing it exists.
  if (hit.type === "company" && hit.is_partner === true) {
    marks.push(
      <Badge key="partner">{t("search.partner.badge")}</Badge>,
      <a
        key="partner-open"
        className="entity-link"
        href={routeHash(companyTabRoute(hit.id, "partner"))}
        aria-label={t("search.partner.openNamed", {
          name: hit.title ?? hit.id,
        })}
      >
        {t("search.partner.open")}
      </a>,
    );
  }
  return marks;
}
