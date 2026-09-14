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
import { api } from "../api/client";
import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import type { BoardDeal } from "../design-system/composed";
import { EmailReference } from "../design-system/emailreference";
import { Eyebrow } from "../design-system/eyebrow";
import { formatElapsed } from "../format/now";
import { useLocale, useT } from "../i18n";
import { QueryStates, throwProblem } from "./common";
import "./dealmailaside.css";

type Activity = components["schemas"]["Activity"];

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
      <a className="dealmail__all" href={href}>
        {t("deal.mail.viewAll")}
      </a>
    </div>
  );
}

/**
 * The board's binding: the aside for one card, addressed to the deal it is
 * about. Built HERE rather than in the board's props because this is the tier
 * that holds routes, and the board is a 4,000-line screen already.
 */
export function dealMailAside(deal: BoardDeal) {
  return (
    <DealMailAside
      dealId={deal.id}
      href={routeHash({ screen: "deals", id: deal.id })}
    />
  );
}
