// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ENTITY, ENTITY_KINDS, type EntityKind } from "../app/entity";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Badge, Card, EmptyState, SearchField } from "../design-system/atoms";
import { EmailEntry } from "../design-system/emailentry";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryGate, throwProblem } from "./common";
import { useOpenEmail } from "./openemail";
import "./search.css";

type SearchResult = components["schemas"]["SearchResult"];

// RS-1/RS-2: the cross-object search results screen. Hits are grouped by
// record type (fixed display order below) so a caller scanning "acme" sees
// people, companies, deals, activities, and leads as separate sections
// rather than one undifferentiated ranked list.
const GROUP_ORDER = [
  "person",
  "organization",
  "deal",
  "activity",
  "lead",
  // Last: a tag is a WORD, and somebody searching a name is usually after the
  // records rather than the label. It sits below them and is how they get to
  // the rest of the slice.
  "tag",
] as const;
const GROUP_KEY: Record<string, MessageKey> = {
  person: "search.group.person",
  organization: "search.group.organization",
  deal: "search.group.deal",
  activity: "search.group.activity",
  lead: "search.group.lead",
  tag: "search.group.tag",
};
// Only these hit types have a 360 to route to (the app-wide ENTITY registry).
// `activity` is a valid SearchResult type but has no record route, so it
// renders as plain text instead of an EntityRef link.
const LINKABLE_KINDS = new Set<EntityKind>(ENTITY_KINDS);

export function SearchScreen({ q }: Readonly<{ q: string }>) {
  const t = useT();
  const [draft, setDraft] = useState(q);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const zone = useRecordZone();
  const query = useQuery({
    queryKey: ["search", q],
    enabled: q.trim().length > 0,
    queryFn: async () => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q, limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

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
        <QueryGate query={query}>
          {(data) =>
            data.data.length === 0 ? (
              <EmptyState>{t("search.empty", { q })}</EmptyState>
            ) : (
              <SearchGroups results={data.data} onOpenEmail={setOpenEmail} />
            )
          }
        </QueryGate>
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

function SearchGroups({
  results,
  onOpenEmail,
}: Readonly<{
  results: SearchResult[];
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  return (
    <div className="search-groups arrive-stack">
      {GROUP_ORDER.filter((type) => results.some((r) => r.type === type)).map(
        (type) => (
          <Card key={type} className="search-group">
            <h2 className="t-label">{t(GROUP_KEY[type])}</h2>
            <ul className="search-hits">
              {results
                .filter((r) => r.type === type)
                .map((hit) => (
                  <SearchHit
                    key={`${hit.type}:${hit.id}`}
                    hit={hit}
                    onOpenEmail={onOpenEmail}
                  />
                ))}
            </ul>
          </Card>
        ),
      )}
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
  // A tag is not an ENTITY kind — it has no 360 — but it does have a page, and
  // it is the whole point of finding one: the word is the way to the records
  // carrying it. Routed on its own rather than by widening ENTITY_KINDS, which
  // drives record routing everywhere else and would claim a tag is a record.
  const isTag = hit.type === "tag";
  const isLinkable = LINKABLE_KINDS.has(hit.type as EntityKind);
  return (
    <li className="search-hit">
      <div className="search-hit-title">
        {isTag ? (
          <button
            type="button"
            className="entity-link"
            onClick={() => navigate({ screen: "tags", id: hit.id })}
          >
            {hit.title ?? hit.id}
          </button>
        ) : isLinkable ? (
          // The search API already returns the hit's display name as
          // `title` — routing through EntityRef here would re-fetch the
          // same record per hit (an N+1 GET per result) just to re-derive
          // a name we already have.
          <button
            type="button"
            className="entity-link"
            onClick={() =>
              navigate(ENTITY[hit.type as EntityKind].route(hit.id))
            }
          >
            {hit.title ?? hit.id}
          </button>
        ) : (
          <span>{hit.title ?? hit.id}</span>
        )}
        {/* Only a tier the reader would not otherwise assume. In native mode
            every stored record is `authoritative` (contract: external and
            unverified are reserved for overlay/connector rows), so badging it
            put the same green pill on every hit on the page — a mark that
            never varies marks nothing, and it crowded out the one that does.
            `unverified` is the opposite case and keeps its badge: it is rare by
            the same contract, and a tier the record CARRIES while the page draws
            nothing reads as a record with nothing to declare.

            Neither badge names the SYSTEM a row came from. `external` covers
            every overlay- and connector-sourced row and the hit carries no
            provider field, so one vendor's name would be stamped on rows
            mirrored from any other. */}
        {hit.trust_tier === "external" && (
          <Badge tone="accent">{t("search.tier.mirrored")}</Badge>
        )}
        {hit.trust_tier === "unverified" && (
          <Badge tone="warn">{t("search.tier.unverified")}</Badge>
        )}
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
        <p className="search-hit-snippet">
          {t("search.tag.carriedBy", {
            count: formatNumber(hit.carried_by, locale),
          })}
        </p>
      )}
      {hit.snippet && <p className="search-hit-snippet">“{hit.snippet}”</p>}
    </li>
  );
}
