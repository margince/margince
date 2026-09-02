// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Building2, Contact, Handshake } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import { api } from "../api/client";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelRow } from "../design-system/panel";
import { TagPill } from "../design-system/tagpill";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import "./tagresult.css";

/**
 * The page behind every tag pill: what carries this word, grouped by type.
 *
 * A preview, not a list. Two rows per group and a link into the record view
 * the reader already knows — building a fourth table here would be a second
 * answer to a question three screens answer well, and it would drift from them
 * the first time a column changed.
 */
export function TagResultScreen({ tagID }: Readonly<{ tagID?: string }>) {
  const t = useT();
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
    return <p className="tagresult-note">{t("tagResult.gone")}</p>;
  }

  const usage = tag.data.usage;
  const total = usage.people + usage.companies + usage.deals;

  return (
    <div className="tagresult">
      <header className="tagresult-head">
        <TagPill
          name={tag.data.name}
          tone={tag.data.color}
          archived={Boolean(tag.data.archived_at)}
        />
        <span className="t-small">
          {t("tagResult.totalVisible", { count: String(total) })}
        </span>
      </header>
      {tag.data.description && (
        <p className="tagresult-note">{tag.data.description}</p>
      )}
      <div className="tagresult-grid">
        <ResultGroup
          title={t("tagResult.people")}
          icon={Contact}
          count={usage.people}
          href={`#/contacts?tag_id=${tagID}`}
        />
        <ResultGroup
          title={t("tagResult.companies")}
          icon={Building2}
          count={usage.companies}
          href={`#/companies?tag_id=${tagID}`}
        />
        <ResultGroup
          title={t("tagResult.deals")}
          icon={Handshake}
          count={usage.deals}
          href={`#/deals?tag_id=${tagID}`}
        />
      </div>
    </div>
  );
}

/**
 * One record type's share of a tag: how many carry it, and the way in.
 *
 * The count comes from the tag read rather than from a page of rows, because a
 * reader deciding whether to open a group needs the size before the rows —
 * and a group of zero says so without a request nobody needs.
 */
function ResultGroup({
  title,
  icon: Icon,
  count,
  href,
}: Readonly<{
  title: string;
  icon: LucideIcon;
  count: number;
  href: string;
}>) {
  const t = useT();
  return (
    <Panel
      title={title}
      titleAction={<Badge>{String(count)}</Badge>}
      footer={
        count > 0 ? (
          <Button small variant="ghost" onClick={() => (window.location.hash = href.slice(1))}>
            {t("tagResult.viewAll", { count: String(count), kind: title })}
          </Button>
        ) : undefined
      }
    >
      <PanelRow>
        <span className="tagresult-row">
          <Icon aria-hidden />
          <span>
            {count > 0
              ? t("tagResult.carry", { count: String(count) })
              : t("tagResult.none")}
          </span>
        </span>
      </PanelRow>
    </Panel>
  );
}
