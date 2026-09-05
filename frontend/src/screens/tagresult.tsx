// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { LucideIcon } from "lucide-react";
import { Building2, Contact, Handshake } from "lucide-react";

import { api } from "../api/client";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { isTagTone } from "../design-system/tagpill";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import "./tagresult.css";

/**
 * The page behind every tag pill: the records carrying this word, named.
 *
 * A RESULT LIST, which is what a reader arriving from a search hit is looking
 * for. It once drew three cards reporting counts — "1 carry this tag" — which
 * answered how many and never which, so the one question the page exists for
 * needed three more clicks to answer.
 *
 * The rows are the real records, read through the same `tag_id` filter the
 * three record lists offer, so this page and "View all" cannot disagree about
 * what carries the word. Each group shows the first few and hands the rest to
 * the list screen that already pages, sorts and filters them properly — the
 * page names records, it does not become a fourth table.
 */
const PREVIEW_ROWS = 5;

export function TagResultScreen({ tagID }: Readonly<{ tagID?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const tag = useQuery({
    queryKey: ["tag", tagID],
    enabled: Boolean(tagID),
    queryFn: async () => {
      const { data, error } = await api.GET("/tags/{id}", {
        params: { path: { id: tagID as string } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  if (!tagID || tag.isPending) {
    return null;
  }
  if (tag.isError) {
    // A merged-away or deleted tag: the pill that led here outlived the word.
    return <p className="wrap tagresult-note">{t("tagResult.gone")}</p>;
  }

  const usage = tag.data.usage;
  const total = usage.people + usage.companies + usage.deals;

  return (
    <div className="wrap tagresult">
      {/* The page heads ITSELF, with the word. The shell steps aside for this
          screen (SELF_HEADED_SCREENS) because it cannot know a tag's name, and
          "Tag" above a pill spelling the name is the page named twice.

          The name is heading TEXT rather than a pill: a pill carries its own
          small type, so a page title drawn as one renders chip-sized. The
          tag's colour still reads, as the pill's own dot. */}
      <header className="tagresult-head">
        <h1 className="tagresult-title">
          {isTagTone(tag.data.color) && !tag.data.archived_at && (
            <span
              className={`tagpill-dot tagpill-dot-${tag.data.color}`}
              aria-hidden
            />
          )}
          {tag.data.name}
          {tag.data.archived_at && (
            <span className="tagresult-retired t-sub">
              {t("tags.archived")}
            </span>
          )}
        </h1>
        {/* Not drawn for a RETIRED word. The usage total counts assignments
            that still exist, while every record list requires the tag to be
            live — so on a retired tag the sentence is the one line on the page
            still promising rows the groups below it correctly do not show. */}
        {!tag.data.archived_at && (
          <span className="t-caption">
            {t("tagResult.totalVisible", {
              count: formatNumber(total, locale),
            })}
          </span>
        )}
      </header>
      {tag.data.description && (
        <p className="tagresult-note">{tag.data.description}</p>
      )}
      {total === 0 ? (
        <Panel title={t("tagResult.resultsTitle")}>
          <PanelBody>
            <p className="tagresult-note">{t("tagResult.nothingCarries")}</p>
          </PanelBody>
        </Panel>
      ) : (
        <div className="tagresult-groups">
          <ResultGroup
            kind="person"
            title={t("tagResult.people")}
            icon={Contact}
            count={usage.people}
            tagID={tagID}
          />
          <ResultGroup
            kind="organization"
            title={t("tagResult.companies")}
            icon={Building2}
            count={usage.companies}
            tagID={tagID}
          />
          <ResultGroup
            kind="deal"
            title={t("tagResult.deals")}
            icon={Handshake}
            count={usage.deals}
            tagID={tagID}
          />
        </div>
      )}
    </div>
  );
}

/** What each record type is called on the wire, and where its rows live. */
const GROUPS = {
  person: { path: "/people", screen: "contacts" },
  organization: { path: "/organizations", screen: "companies" },
  deal: { path: "/deals", screen: "deals" },
} as const;

// "View all" carries a FILTER, and a filter is not part of a Route — the type
// deliberately holds screen and ids only, and the list screens read `tag_id`
// off the address. So the whole-list link is written as an address rather than
// navigated as a route; `tagfilter.ts` owns the parameter's spelling.
function allRecordsHref(screen: string, tagID: string): string {
  return `#/${screen}?tag_id=${encodeURIComponent(tagID)}`;
}

type GroupKind = keyof typeof GROUPS;

/**
 * One record type's rows: the records themselves, by name, each opening its
 * own page.
 *
 * A group the tag is not on draws nothing at all rather than a card reporting
 * zero. The count came from the tag read, so an absent group costs no request —
 * and three cards, two of them saying "none", is the shape this page had when
 * it told a reader nothing.
 */
function ResultGroup({
  kind,
  title,
  icon: Icon,
  count,
  tagID,
}: Readonly<{
  kind: GroupKind;
  title: string;
  icon: LucideIcon;
  count: number;
  tagID: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const group = GROUPS[kind];
  const rows = useQuery({
    queryKey: ["tag-records", kind, tagID],
    // The tag read already said this type carries none, so asking for its rows
    // is a round trip whose answer is known. The hook still RUNS — a group that
    // called it conditionally would change the hook order as counts arrive.
    enabled: count > 0,
    queryFn: async () => {
      const { data, error } = await api.GET(group.path, {
        params: { query: { tag_id: [tagID], limit: PREVIEW_ROWS } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data ?? [];
    },
  });

  if (count === 0) {
    return null;
  }

  const listed = rows.data ?? [];
  // The HEADER counts the rows, not the tag's usage. The two are answers to
  // different questions and they disagree in a state a reader can reach: the
  // usage count admits a retired tag, while the list filter every record screen
  // uses requires the tag to be live. A retired word would head "People (2)"
  // over an empty group. Once more rows exist than the preview shows, the total
  // is the honest ceiling and the footer says so.
  const shown = formatNumber(
    listed.length < PREVIEW_ROWS ? listed.length : count,
    locale,
  );
  return (
    <Panel title={rows.isSuccess ? `${title} (${shown})` : title}>
      <PanelBody>
        <SurfaceState
          label={undefined}
          state={
            rows.isPending
              ? "loading"
              : rows.isError
                ? "unavailable"
                : listed.length > 0
                  ? "ready"
                  : "empty"
          }
          loadingLabel={t("tagResult.loadingRows", { kind: title })}
          loadingLines={Math.min(count, PREVIEW_ROWS)}
          emptyLabel={t("tagResult.noneLeft")}
        >
          <ul className="tagresult-rows">
            {listed.map((row) => (
              <li key={row.id}>
                <button
                  type="button"
                  className="tagresult-row"
                  onClick={() => navigate({ screen: group.screen, id: row.id })}
                >
                  <Icon aria-hidden />
                  <span className="tagresult-name">{recordName(row, t)}</span>
                </button>
              </li>
            ))}
          </ul>
        </SurfaceState>
        {/* Offered only when the preview is FULL: a group showing every row it
            has needs no way to see more, and the tag's own count cannot be the
            test here for the same reason it is not the header. */}
        {listed.length >= PREVIEW_ROWS && (
          <Button
            small
            variant="ghost"
            onClick={() => {
              window.location.hash = allRecordsHref(group.screen, tagID).slice(
                1,
              );
            }}
          >
            {t("tagResult.viewAll", { count: shown, kind: title })}
          </Button>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * What to call one record.
 *
 * The three types name themselves differently on the wire — a person carries
 * `full_name`, a company `display_name`, a deal `name` — and a row whose name
 * is empty is named as unnamed rather than rendered as a blank line nobody can
 * press with confidence.
 */
function recordName(
  row: Readonly<Record<string, unknown>>,
  t: ReturnType<typeof useT>,
): string {
  for (const field of ["full_name", "display_name", "name"]) {
    const value = row[field];
    if (typeof value === "string" && value.trim() !== "") {
      return value;
    }
  }
  return t("tagResult.unnamed");
}
