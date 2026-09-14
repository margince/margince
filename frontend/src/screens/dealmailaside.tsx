// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The flyout under a board card's mail line: the deal's last few emails, for a
// rep who wants the subjects without opening the deal.
//
// Read from the timeline the way every other surface reads it — the same
// `/activities` narrowing the record page makes — so the rows here are the
// rows the record page shows first, withheld ones included. A message this
// reader may discover but not read is cited in the same words the timeline
// uses for it, and the card's own date can sit BEHIND the newest row here:
// the card counts only what the whole workspace may see (Deal.last_email says
// why), while this list is what THIS reader may know about.
//
// Mounted only while the flyout is open (Popover renders its panel on demand),
// so a board of a hundred cards asks for nothing until a pointer settles.

import { useQuery } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import type { BoardDeal } from "../design-system/composed";
import { DealMailChip } from "../design-system/dealcard";
import { EmailReference } from "../design-system/emailreference";
import { Eyebrow } from "../design-system/eyebrow";
import type { ListColumn } from "../design-system/listtable";
import { formatElapsed } from "../format/now";
import { type Translator, useLocale, useT } from "../i18n";
import { boardMail } from "./boarddeal";
import { QueryStates, throwProblem } from "./common";
import "./dealmailaside.css";

type Activity = components["schemas"]["Activity"];
type Deal = components["schemas"]["Deal"];

// Three, like the card the flyout was modelled on: enough to read the shape of
// the exchange, few enough that the panel stays an aside rather than a page.
const MAIL_ASIDE_ROWS = 3;

function useDealMail(dealId: string) {
  return useQuery({
    // Under the deal's timeline prefix, so logging or sending mail from
    // anywhere refreshes this the way it refreshes the record page
    // (activitykeys.ts) — a flyout still citing the mail before the one just
    // sent reads as a broken send.
    queryKey: ["activities", "deal", dealId, "mail-aside"],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: {
          query: {
            entity_type: "deal",
            entity_id: dealId,
            kind: "email",
            limit: MAIL_ASIDE_ROWS,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function DealMailAside({
  dealId,
  href,
}: Readonly<{
  dealId: string;
  /** The deal's own address, for the reader who wants the whole timeline. */
  href: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useDealMail(dealId);
  const now = Date.now();
  const when = (mail: Activity) => {
    const ago = formatElapsed(
      now - new Date(mail.occurred_at).getTime(),
      t,
      locale,
    );
    if (mail.direction === "outbound") {
      return t("deal.mail.sent", { ago });
    }
    if (mail.direction === "inbound") {
      return t("deal.mail.received", { ago });
    }
    return ago;
  };
  return (
    <div className="dealmail">
      <Eyebrow as="h3" className="dealmail__title">
        {t("deal.mail.title")}
      </Eyebrow>
      <QueryStates query={query} pendingLabel={t("deal.mail.title")}>
        {query.data?.data?.length ? (
          <ul className="dealmail__list">
            {query.data.data.map((mail) => (
              <li key={mail.id}>
                <EmailReference
                  subject={mail.subject}
                  occurredAt={when(mail)}
                  withheld={mail.content_state === "withheld"}
                  stacked
                />
              </li>
            ))}
          </ul>
        ) : (
          // The card only draws its mail line when the workspace has mail on
          // the deal, but this list is the READER's — and a reader outside
          // every row's audience is one the timeline still answers, with
          // withheld rows. An empty page is therefore rare rather than
          // impossible, and it says so instead of drawing a blank aside.
          <p className="dealmail__none">{t("deal.mail.none")}</p>
        )}
      </QueryStates>
      <a className="link-button dealmail__all" href={href}>
        {t("deal.mail.viewAll")}
        <ArrowRight aria-hidden="true" />
      </a>
    </div>
  );
}

/**
 * The binding: the aside for one card or one row, addressed to the deal it is
 * about. Built HERE rather than in the board's props because this is the tier
 * that holds routes, and the deals screen is a 4,000-line file already. It
 * asks for the id alone, so a board card and a table row — different shapes
 * of the same deal — both fit.
 */
export function dealMailAside(deal: Pick<BoardDeal, "id">) {
  return (
    <DealMailAside
      dealId={deal.id}
      href={routeHash({ screen: "deals", id: deal.id })}
    />
  );
}

/**
 * The table's column: the same chip and the same flyout the card draws, off
 * the same wire field, so a reader switching views reads one fact once. No
 * `sort` — the field is attached to the page after the query, not a column
 * the server can order by, and a header that promised an ordering it cannot
 * give would be a control that does nothing.
 */
export function lastMailColumn(t: Translator): ListColumn<Deal> {
  return {
    key: "last_mail",
    header: t("deal.lastMail"),
    cell: (deal) => {
      const mail = boardMail(deal.last_email);
      return mail ? (
        <DealMailChip mail={mail} aside={dealMailAside(deal)} />
      ) : (
        // A word rather than the card's blank: a column is scanned down, and
        // an empty cell in one cannot be told from a cell still loading.
        <span className="t-caption">{t("deals.lastMailNone")}</span>
      );
    },
  };
}
