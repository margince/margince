import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Card } from "../design-system/atoms";
import { EvidenceMark } from "../design-system/evidencemark";
import { Eyebrow } from "../design-system/eyebrow";
import { SurfaceState } from "../design-system/surfacestate";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryGate, throwProblem } from "./common";
import { derivedSource } from "./evidencesource";

type OrganizationFact = components["schemas"]["OrganizationFact"];
type TechnicalEnrichLane = components["schemas"]["TechnicalEnrichLane"];

/**
 * The fact fields a technical lookup owns, grouped as a reader thinks about
 * them: how they get mail, what their site is built with, what they operate,
 * where it runs.
 *
 * Partitioned by FIELD and never by source, which looks equivalent and is not:
 * a person correcting a machine-read value rewrites the row's source to
 * `human`, and a source-partitioned card would drop exactly the rows somebody
 * cared enough to fix.
 */
const TECHNICAL_SECTIONS: readonly {
  heading: MessageKey;
  fields: readonly string[];
}[] = [
  { heading: "co.tech.mail", fields: ["mail_provider", "email_security"] },
  { heading: "co.tech.web", fields: ["technology"] },
  { heading: "co.tech.services", fields: ["operated_service"] },
  { heading: "co.tech.hosting", fields: ["hosting_provider"] },
];

/** TECHNICAL_FIELDS is every field this card claims, so the general evidence
 * card can exclude them and no fact renders twice. */
export const TECHNICAL_FIELDS: readonly string[] = TECHNICAL_SECTIONS.flatMap(
  (section) => section.fields,
);

/** isTechnicalFact reports whether a fact belongs to the technical profile. */
export function isTechnicalFact(fact: OrganizationFact): boolean {
  return fact.category === "signal" && TECHNICAL_FIELDS.includes(fact.field);
}

/**
 * TechnicalProfileCard shows what a company publicly runs.
 *
 * It reads and never asks: the lookup is queued by the site read, so this card
 * has no button. A reader who wants it refreshed reads the site.
 *
 * Every value carries the public record that proved it — the MX host, the
 * certificate hostname, the matched marker — through the same evidence mark
 * the rest of the record uses, because "how do you know?" is the first
 * question a claim like this invites.
 */
export function TechnicalProfileCard({
  orgId,
}: Readonly<{ orgId: string }>): ReactNode {
  const t = useT();

  const facts = useQuery({
    queryKey: ["org-facts", orgId],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}/facts", {
        params: { path: { id: orgId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data ?? [];
    },
  });

  // 404 is the honest "never looked up", and leaves the card empty rather
  // than reporting a failure nobody caused.
  //
  // Read once, not polled. The lookup is queued by the site read that
  // preceded it, so by the time a reader is looking at this card the work is
  // either done or running in a worker; a card that polled would be watching
  // for an event it did not start.
  const lanes = useQuery({
    queryKey: ["org-technical-latest", orgId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/organizations/{id}/technical-enrich/latest",
        { params: { path: { id: orgId } } },
      );
      if (response.status === 404) {
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data ?? null;
    },
  });

  return (
    <Card title={t("co.tech.title")} sub={t("co.tech.sub")}>
      <QueryGate query={facts} pendingLabel={t("co.tech.title")}>
        {(rows) => (
          <TechnicalSections
            facts={rows.filter(isTechnicalFact)}
            lanes={lanes.data?.lanes ?? []}
          />
        )}
      </QueryGate>
    </Card>
  );
}

/** TechnicalSections draws the four groups, and says plainly when there is
 * nothing rather than rendering four empty headings. */
function TechnicalSections({
  facts,
  lanes,
}: Readonly<{
  facts: readonly OrganizationFact[];
  lanes: readonly TechnicalEnrichLane[];
}>): ReactNode {
  const t = useT();
  if (facts.length === 0) {
    return (
      <SurfaceState
        state="empty"
        emptyLabel={t("co.tech.empty")}
        loadingLabel={t("co.tech.title")}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <>
      {TECHNICAL_SECTIONS.map((section) => {
        const rows = facts.filter((fact) =>
          section.fields.includes(fact.field),
        );
        if (rows.length === 0) {
          return null;
        }
        return (
          <div key={section.heading} className="co-facts-group">
            <Eyebrow as="h3">{t(section.heading)}</Eyebrow>
            {rows.map((fact) => (
              <TechnicalRow
                key={`${fact.field}:${fact.value_key}`}
                fact={fact}
              />
            ))}
          </div>
        );
      })}
      <LaneNotices lanes={lanes} />
    </>
  );
}

/** One technical value, with the public record that proved it. */
function TechnicalRow({
  fact,
}: Readonly<{ fact: OrganizationFact }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <div className="co-field">
      <span className="t-label">{t(technicalFieldLabel(fact.field))}</span>
      <div>
        <EvidenceMark
          value={fact.value}
          source={derivedSource(fact, locale, recordZone)}
        />
      </div>
    </div>
  );
}

/** technicalFieldLabel names one technical field in the reader's language. */
function technicalFieldLabel(field: string): MessageKey {
  switch (field) {
    case "mail_provider":
      return "co.factField.mail_provider";
    case "email_security":
      return "co.factField.email_security";
    case "hosting_provider":
      return "co.factField.hosting_provider";
    case "operated_service":
      return "co.factField.operated_service";
    default:
      return "co.factField.technology";
  }
}

/**
 * LaneNotices says which sources did not answer.
 *
 * Worth a line because of what it means: a lane that failed left what it read
 * last time alone, so part of this card can be a week older than the rest, and
 * a reader deciding whether to trust "no webshop" deserves to know the
 * certificate log has been down.
 */
function LaneNotices({
  lanes,
}: Readonly<{ lanes: readonly TechnicalEnrichLane[] }>): ReactNode {
  const t = useT();
  const notices = lanes.filter(
    (lane) => lane.outcome === "failed" || lane.outcome === "refused",
  );
  if (notices.length === 0) {
    return null;
  }
  return (
    <div className="co-facts-group">
      {notices.map((lane) => (
        <p key={lane.lane} className="t-caption">
          <Badge tone="warn" quiet>
            {t(laneLabel(lane.lane))}
          </Badge>{" "}
          {lane.outcome === "refused"
            ? t("co.tech.laneRefused")
            : t("co.tech.laneFailed", { lane: t(laneLabel(lane.lane)) })}
        </p>
      ))}
    </div>
  );
}

function laneLabel(lane: TechnicalEnrichLane["lane"]): MessageKey {
  switch (lane) {
    case "dns":
      return "co.tech.lane.dns";
    case "certlog":
      return "co.tech.lane.certlog";
    default:
      return "co.tech.lane.homepage";
  }
}
