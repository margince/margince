// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Avatar } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import type { ReviewRow } from "./company-review-state";
import { ProfileArticle } from "./profile-digest-article";
import {
  citationOf,
  citationsOf,
  type Fact,
  factsByCategory,
  hostOf,
  type LegalEntity,
  type Page,
  type Person,
} from "./profile-digest-data";
import { DigestLine } from "./profile-digest-lines";
import { ProfileSidebar } from "./profile-digest-sidebar";
import "./profile-digest.css";

/**
 * The profile, in two faces the deck's flow needs at different moments.
 *
 * WITHOUT `read`, this is the deck's own companion: a single narrow column of
 * record lines, the field being decided marked in it, beside the deck that is
 * still writing them. Answer a card and you watch the line it belongs to
 * land — that is the whole reason it sits there, so it stays a plain list
 * rather than growing the sections or the sidebar below.
 *
 * WITH `read`, this is the whole record, read as the two-column document a
 * reader asked to see: a header naming the company and what stands open, the
 * record grouped into sections down the left, and a sidebar of the facts a
 * reader scans without opening a section on the right. `read` carries the
 * crawl's own findings beyond the fields the deck ever asks about — the
 * facts, the people, the legal entities — which is what the sections beyond
 * the record's own four groups draw from.
 *
 * EVERY LINE CARRIES ITS SOURCE, numbered the way an encyclopedia numbers
 * them: the same page cited twice gets the same number, and the pages
 * themselves are listed once at the foot. A line with no source says so
 * instead of borrowing one — something typed by a person, or carried over
 * from a profile that already existed, has no page to cite.
 *
 * AN UNANSWERED LINE IN THE DOCUMENT IS AN ACTION: `onSettle` takes the
 * reader to that field's card in the deck. The companion never renders that
 * action — the deck asking about the same field is already the surface it is
 * beside, so a button pointed at the card behind it would aim at itself.
 *
 * A WRITTEN LINE IN THE DOCUMENT IS EDITABLE where it stands (`onField`), and
 * the companion carries the door to the document (`onReadWhole`) in its own
 * head: the record is what a reader wants to read whole, so the way to it
 * sits on the record rather than in the deck's tray two panels away.
 */

/** What the site read carries beyond the profile fields the deck asks about. */
export type ProfileDigestRead = Readonly<{
  root_url: string;
  pages: readonly Page[];
  facts: readonly Fact[];
  people: readonly Person[];
  legal_entities?: readonly LegalEntity[];
}>;

export function ProfileDigest({
  rows,
  active,
  read,
  identity,
  onSettle,
  onField,
  onReadWhole,
}: Readonly<{
  rows: readonly ReviewRow[];
  /** The field the deck is asking about, marked in the companion as it is
   * decided. Meaningless in the document face, which has no single field in
   * front of the reader at once. */
  active?: CompanyFieldName;
  /** The rest of the crawl, and the document layout, shown only where a
   * reader asked for the whole record. */
  read?: ProfileDigestRead;
  /** Whose record this is, for the mark at the head of either face: the site
   * it was read from, and the logo the read resolved when it found one. */
  identity?: ProfileIdentity;
  /** Where "Settle it" on an unanswered line goes. Only read in the document
   * face — see the docblock above for why the companion never wires it. */
  onSettle?: (field: CompanyFieldName) => void;
  /** Lets the document's written lines be corrected where they stand. */
  onField?: (field: CompanyFieldName, value: string) => void;
  /** The companion's door to the whole record. */
  onReadWhole?: () => void;
}>) {
  if (read === undefined) {
    return (
      <DigestCompanion
        rows={rows}
        active={active}
        identity={identity}
        onReadWhole={onReadWhole}
      />
    );
  }
  return (
    <DigestDocument
      rows={rows}
      read={read}
      identity={identity}
      onSettle={onSettle}
      onField={onField}
    />
  );
}

/** The company as the head of the record shows it. */
export type ProfileIdentity = Readonly<{
  /** The site the record was read from; what the monogram's tint derives from. */
  rootUrl: string;
  /** Where the mark the read resolved is served from, when it found one. */
  logoUrl?: string;
}>;

// The record's mark: the logo the read resolved from the company's own site,
// or the deterministic monogram every other surface draws for it. Named from
// the record's own display name, so what the reader corrected shows.
function ProfileMark({
  rows,
  identity,
}: Readonly<{ rows: readonly ReviewRow[]; identity?: ProfileIdentity }>) {
  const name =
    rows.find((row) => row.field === "display_name")?.value.trim() ||
    (identity === undefined ? "" : hostOf(identity.rootUrl));
  return (
    <Avatar
      shape="organization"
      size="md"
      name={name}
      identity={identity?.rootUrl}
      src={identity?.logoUrl}
    />
  );
}

// The companion: the record's own lines, in the record's own order, nothing
// beyond it — see the top-level docblock for why it stays this plain.
function DigestCompanion({
  rows,
  active,
  identity,
  onReadWhole,
}: Readonly<{
  rows: readonly ReviewRow[];
  active?: CompanyFieldName;
  identity?: ProfileIdentity;
  onReadWhole?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const cites = citationsOf(rows);
  const number = new Map(cites.map((cite) => [cite.url, cite.n]));
  const written = rows.filter((row) => row.value.trim() !== "").length;

  return (
    <aside className="pdigest">
      <div className="pdigest-head">
        <ProfileMark rows={rows} identity={identity} />
        <div className="pdigest-head-text">
          <Eyebrow className="pdigest-eyebrow">{t("ob.digest.where")}</Eyebrow>
          <p className="pdigest-count">
            {t("ob.digest.written", {
              n: formatNumber(written, locale),
              m: formatNumber(rows.length, locale),
            })}
          </p>
        </div>
        {onReadWhole === undefined ? null : (
          <button type="button" className="pdigest-whole" onClick={onReadWhole}>
            {t("ob.deck.readWhole")}
          </button>
        )}
      </div>
      <div className="pdigest-body">
        {rows.map((row) => (
          <DigestLine
            key={row.field}
            row={row}
            n={citationOf(row, number)}
            active={row.field === active}
          />
        ))}
      </div>
    </aside>
  );
}

// The document: header, article, sidebar — the whole record a reader asked
// to see.
function DigestDocument({
  rows,
  read,
  identity,
  onSettle,
  onField,
}: Readonly<{
  rows: readonly ReviewRow[];
  read: ProfileDigestRead;
  identity?: ProfileIdentity;
  onSettle?: (field: CompanyFieldName) => void;
  onField?: (field: CompanyFieldName, value: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const factGroups = factsByCategory(read.facts);
  const legalEntities = read.legal_entities ?? [];
  const people = read.people;
  // Rendering order, so a URL first seen backing a legal entity — the
  // article's Identity section folds those in right after the record's own
  // legal-identity lines — never gets bumped behind a fact or a person cited
  // further down the page.
  const extra = [
    ...legalEntities.map((entity) => entity.source_url),
    ...factGroups.flatMap((group) => group.facts.map((f) => f.evidence_url)),
    ...people.map((p) => p.evidence_url),
  ];
  const cites = citationsOf(rows, extra);
  const number = new Map(cites.map((cite) => [cite.url, cite.n]));
  // Cited, not merely written: a row a person typed or carried in from an
  // existing profile has a value and no page behind it, and counting it here
  // would claim a citation the line itself does not print.
  const citedCount = rows.filter(
    (row) => row.evidence !== null && row.evidence.source !== "",
  ).length;
  // The same predicate the article's own unanswered rows render on, so the
  // header's count and the dashed rows on the page can never disagree.
  const openCount = rows.filter((row) => row.value.trim() === "").length;
  const companyName =
    rows.find((row) => row.field === "display_name")?.value.trim() ||
    hostOf(read.root_url);
  const host = hostOf(read.root_url);

  return (
    <div className="pdigest-doc">
      <header className="pdigest-hero">
        <Eyebrow as="h2" className="pdigest-hero-eyebrow">
          {t("ob.digest.where")}
        </Eyebrow>
        <div className="pdigest-hero-row">
          <div className="pdigest-hero-identity">
            <ProfileMark
              rows={rows}
              identity={identity ?? { rootUrl: read.root_url }}
            />
            <div>
              <p className="pdigest-hero-name">{companyName}</p>
              <p className="pdigest-hero-sub">
                {t("ob.digest.companyLine", {
                  n: formatNumber(read.pages.length, locale),
                  host,
                })}
              </p>
            </div>
          </div>
          <div className="pdigest-hero-stats">
            <div className="pdigest-figure">
              <strong className="pdigest-figure-value">
                {formatNumber(citedCount, locale)}
              </strong>
              <span className="pdigest-figure-caption t-caption">
                {t("ob.digest.citedCaption")}
              </span>
            </div>
            <div className="pdigest-figure" data-warn={openCount > 0}>
              <strong className="pdigest-figure-value">
                {formatNumber(openCount, locale)}
              </strong>
              <span className="pdigest-figure-caption t-caption">
                {t("ob.digest.openCaption")}
              </span>
            </div>
          </div>
        </div>
      </header>
      <div className="pdigest-grid">
        <ProfileArticle
          rows={rows}
          number={number}
          pages={read.pages}
          legalEntities={legalEntities}
          factGroups={factGroups}
          people={people}
          cites={cites}
          onSettle={onSettle}
          onField={onField}
        />
        <ProfileSidebar
          rows={rows}
          number={number}
          facts={read.facts}
          legalEntities={legalEntities}
          rootUrl={read.root_url}
        />
      </div>
    </div>
  );
}
