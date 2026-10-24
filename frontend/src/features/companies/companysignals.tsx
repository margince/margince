// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { webUrl } from "../../format/weburl";
import { useLocale, useT } from "../../i18n";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { signalKindLabel, signalTone } from "../../lib/recordlabels";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import "./company360.css";

// The account's open signals, on the Tasks tab: commitments, blockers and
// risks are things to act on.

type Signal = components["schemas"]["Signal"];

/**
 * SourcePageLink opens the page a signal was read off, when it names one.
 *
 * The address rides `evidence` rather than a field of its own, because a
 * signal cites its source and a citation is per-claim. `source_type: "page"`
 * is the one kind that is a web address. The others point at rows inside this
 * product, which a reader reaches by other means. So only pages are linked.
 *
 * Renders nothing when no evidence names a page, which is the ordinary case
 * for a signal derived from the product's own records.
 */
function SourcePageLink({ signal }: Readonly<{ signal: Signal }>) {
  const t = useT();
  // Array-checked rather than `?? []`: the client validates no response body.
  // A server ahead of this tab could send a shape `.find` cannot walk. The
  // throw would take the whole account page down over one row's citation.
  const cited = Array.isArray(signal.evidence) ? signal.evidence : [];
  // The first citation that is reachable, not the first one claiming to be a
  // page. A malformed address in the first slot would otherwise hide a good
  // one behind it, and the row would fall silently back to no link.
  const page = cited.find(
    (one) =>
      one?.source_type === "page" &&
      typeof one.source_id === "string" &&
      webUrl(one.source_id) !== null,
  );
  if (!page?.source_id) {
    return null;
  }
  return (
    <OffsiteLink href={page.source_id} className="co-rowlink co-signal-link">
      {t("co.signals.openSource")}
    </OffsiteLink>
  );
}

/**
 * SignalsSection reads the account-filtered signals, same endpoint and same
 * withheld/failed handling SignalsCard used. Signals are a separately
 * governed surface, not a 360 section, so this runs its own query rather
 * than reading a slice of `view`.
 */
export function SignalsSection({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const query = useQuery({
    queryKey: ["signals", "company", companyId],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/signals", {
          params: {
            query: { company_id: companyId, status: "open", limit: 10 },
          },
        }),
      );
      return data.data;
    },
  });
  const signals: Signal[] = query.data ?? [];
  const withheld =
    query.isError && problemCodeOf(query.error) === "permission_denied";
  let state: SectionState = "ready";
  if (withheld) {
    state = "withheld";
  } else if (query.isError) {
    state = "failed";
  } else if (query.isPending) {
    state = "loading";
  } else if (signals.length === 0) {
    state = "empty";
  }
  return (
    <Panel title={t("co.signals.title")}>
      {state === "ready" ? (
        signals.map((signal) => (
          <PanelRow key={signal.id} className="co-signal-row">
            <span
              className={`co-dot${signalTone(signal.severity) ? ` co-dot-${signalTone(signal.severity)}` : ""}`}
              aria-hidden="true"
            />
            <span className="co-signal-body">
              <span className="co-signal-title">
                {signalKindLabel(signal.kind, t)}
              </span>
              <span className="co-signal-summary">{signal.summary}</span>
              {/* A signal ABOUT one of the account's projects sends the
                  reader to that project: the summary names it, the link
                  opens it. An account- or contact-subject signal already
                  sits on the page it is about. */}
              {signal.entity_type === "project" && signal.entity_id && (
                <a
                  className="co-rowlink co-signal-link"
                  href={routeHash({ screen: "projects", id: signal.entity_id })}
                >
                  {t("co.signals.openProject")}
                </a>
              )}
              {/* The page the claim was read off. A newsroom signal cites the
                  article rather than copying it, so the headline on this row
                  is the whole of what we hold — without the address, a reader
                  who wants the announcement itself has nowhere to go and the
                  citation proves nothing. */}
              <SourcePageLink signal={signal} />
            </span>
            <span className="co-row-meta t-caption">
              {formatDate(signal.detected_at, locale, recordZone)}
            </span>
          </PanelRow>
        ))
      ) : (
        <PanelBody>
          <SurfaceState
            loadingLabel={t("co.signals.title")}
            state={state}
            emptyLabel={t("co.signals.empty")}
            emptyDetail={t("co.signals.emptyDetail")}
            detail={
              state === "failed" ? { onRetry: () => void query.refetch() } : {}
            }
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}
