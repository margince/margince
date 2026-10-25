// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { EmailEntry } from "../../design-system/emailentry";
import { RecordCard } from "../../design-system/recordcard";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import { companyTabRoute } from "../../lib/companytab";
import { ENTITY } from "../../lib/entity";
import { useRecordZone } from "../../lib/recordzone";
import { navigate, routeHash } from "../../lib/router";
import {
  type SearchRecordCardType,
  searchHitHasCard,
  searchHitRoute,
} from "../../lib/searchkinds";
import type { OpenHit } from "./searchpanels";

type SearchResult = components["schemas"]["SearchResult"];

export function SearchHit({
  hit,
  onOpen,
}: Readonly<{ hit: SearchResult; onOpen: OpenHit }>) {
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
          timestamp={formatActivityWhen(
            summary.occurred_at,
            summary.occurred_precision,
            locale,
            zone,
          )}
          onOpen={() => onOpen(summary.activity_id, "email")}
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
  // Where this kind goes, asked of the one place that knows. An activity has
  // no page, so it opens in the panel beside the email one.
  const isTag = hit.type === "tag";
  const route = searchHitRoute(type, hit.id);
  const opensPanel = !route && type === "activity";
  const go = route
    ? () => navigate(route)
    : opensPanel && (() => onOpen(hit.id, "activity"));
  return (
    <li className="search-hit">
      <div className="search-hit-title">
        {go ? (
          // The hit's own `title`, not EntityRef: that would re-fetch each
          // record (one GET per hit) for a name the search already returned.
          <button
            type="button"
            className="entity-link"
            aria-haspopup={opensPanel ? "dialog" : undefined}
            onClick={go}
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
          a lead's company, a product's sku. Quoting “KAR-9910” claims somebody wrote that
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
