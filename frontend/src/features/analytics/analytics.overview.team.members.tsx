// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure } from "../../design-system/disclosure";
import { Panel, PanelBody } from "../../design-system/panel";
import { forReader } from "../../format/collate";
import { formatDuration, formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { reportingPeriodLabel } from "./reporting.model";

type Review = components["schemas"]["SdrTeamReview"];
type Member = components["schemas"]["SdrTeamMember"];
type Withheld = Review["withheld_figures"][number];

/** Whether the server left a figure out because this reader may not see it. */
export function figureWithheld(review: Review, figure: Withheld): boolean {
  return review.withheld_figures.includes(figure);
}

// A count column. A cell the server sent no figure for stays empty, since the
// table's notes say why; a zero is a measured zero.
function countColumn(
  key: string,
  header: string,
  value: (row: Member) => number | undefined,
  locale: Locale,
): DataTableColumn<Member> {
  return {
    key,
    header,
    align: "end",
    render: (row) => {
      const figure = value(row);
      return figure == null ? "—" : formatNumber(figure, locale);
    },
  };
}

function eventQualifications(review: Review): boolean {
  return (
    review.qualification_basis === "first_qualification" ||
    review.members.some((member) => member.qualified_events != null)
  );
}

function qualificationColumns(
  review: Review,
  t: ReturnType<typeof useT>,
  locale: Locale,
): DataTableColumn<Member>[] {
  const leads = figureWithheld(review, "leads");
  const qualified = leads || figureWithheld(review, "qualified");
  const unanswered = leads || figureWithheld(review, "unanswered");
  const meetings = figureWithheld(review, "meetings");
  const events = eventQualifications(review);
  const targets =
    qualified || figureWithheld(review, "targets")
      ? undefined
      : events
        ? review.member_event_targets
        : review.member_targets;
  const legacy =
    events &&
    !qualified &&
    !figureWithheld(review, "targets") &&
    review.member_targets &&
    review.members.some((row) => row.target != null);
  const targetText = (row: Member, arrival = false) => {
    const target = arrival || !events ? row.target : row.event_target;
    if (!target) return t("analytics.team.noTarget");
    const text = t("analytics.team.targetOf", {
      actual: formatNumber(target.actual, locale),
      target: formatNumber(target.value, locale),
    });
    return !arrival &&
      events &&
      (review.qualification_target_unattributed ?? 0) > 0
      ? t("analytics.team.atLeast", { value: text })
      : text;
  };
  return [
    ...(qualified
      ? []
      : [
          {
            key: "qualified",
            header: events
              ? t("analytics.team.qualified")
              : t("analytics.team.arrivalQualified"),
            align: "end" as const,
            render: (row: Member) => {
              const value = events ? row.qualified_events : row.qualified;
              if (value == null) return t("analytics.readingUnavailable");
              const count = formatNumber(value, locale);
              return events && (review.qualification_unattributed ?? 0) > 0
                ? t("analytics.team.atLeast", { value: count })
                : count;
            },
          },
        ]),
    ...(meetings
      ? []
      : [
          countColumn(
            "meetings",
            t("analytics.team.meetings"),
            (row) => row.meetings_held,
            locale,
          ),
        ]),
    ...(unanswered
      ? []
      : [
          countColumn(
            "unanswered",
            t("analytics.team.unanswered"),
            (row) => row.unanswered,
            locale,
          ),
        ]),
    ...(targets
      ? [
          {
            key: "target",
            header: t("analytics.team.target"),
            align: "end" as const,
            render: (row: Member) => targetText(row),
          },
        ]
      : []),
    ...(legacy
      ? [
          {
            key: "arrivalTarget",
            header: t("analytics.team.arrivalTarget"),
            align: "end" as const,
            render: (row: Member) => targetText(row, true),
          },
        ]
      : []),
  ];
}

function useHandoffColumns(review: Review): DataTableColumn<Member>[] {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const asOf = Date.parse(review.as_of);
  const meetings = figureWithheld(review, "meetings")
    ? []
    : [
        countColumn(
          "meetings",
          t("analytics.team.meetings"),
          (row) => row.meetings_held,
          locale,
        ),
      ];
  if (figureWithheld(review, "handoffs")) return meetings;
  return [
    ...meetings,
    countColumn(
      "submitted",
      t("analytics.team.submitted"),
      (row) => row.submitted,
      locale,
    ),
    countColumn(
      "accepted",
      t("analytics.team.accepted"),
      (row) => row.accepted,
      locale,
    ),
    countColumn(
      "returned",
      t("analytics.team.returned"),
      (row) => row.recycled,
      locale,
    ),
    countColumn(
      "rejected",
      t("analytics.team.rejected"),
      (row) => row.rejected,
      locale,
    ),
    {
      key: "waiting",
      header: t("analytics.team.waiting"),
      align: "end",
      render: (row) => {
        if (row.waiting == null) return "";
        return row.oldest_waiting_since
          ? plural("analytics.team.waitingFor", row.waiting, {
              count: formatNumber(row.waiting, locale),
              age: formatDuration(
                asOf - Date.parse(row.oldest_waiting_since),
                locale,
              ),
            })
          : formatNumber(row.waiting, locale);
      },
    },
  ];
}

// What each column counts, and why the rows are not the team's total. Each
// withheld figure says so in its own words, so no empty column reads as zero.
function memberNotes(
  review: Review,
  zone: string,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string[] {
  const qualification = review.workflow === "qualification";
  const leads = figureWithheld(review, "leads");
  const handoffs = figureWithheld(review, "handoffs");
  const targets = review.member_event_targets ?? review.member_targets;
  const notes = [
    qualification &&
      !leads &&
      t(
        eventQualifications(review)
          ? "analytics.team.eventCredit"
          : "analytics.team.qualificationBasis",
      ),
    !qualification && !handoffs && t("analytics.team.handoffBasis"),
    targets &&
      t("analytics.team.targetBasis", {
        month: reportingPeriodLabel(targets.window, zone, locale),
      }),
    !figureWithheld(review, "meetings") && t("analytics.team.rowsOverlap"),
    t("analytics.team.rowsNotTeam"),
    review.membership_basis === "current" && t("analytics.team.todaysMembers"),
    t("analytics.team.notARanking"),
    review.members_truncated && t("analytics.team.truncated"),
  ];
  return notes.filter((note): note is string => typeof note === "string");
}

function MemberVisibility({ review }: Readonly<{ review: Review }>) {
  const t = useT();
  return (
    <>
      {[
        figureWithheld(review, "meetings") &&
          t("analytics.team.meetingsHidden"),
        figureWithheld(review, "leads") && t("analytics.team.leadsHidden"),
        !figureWithheld(review, "leads") &&
          figureWithheld(review, "qualified") &&
          t("analytics.team.qualifiedHidden"),
        !figureWithheld(review, "leads") &&
          figureWithheld(review, "unanswered") &&
          t("analytics.team.unansweredHidden"),
        figureWithheld(review, "targets") && t("analytics.team.targetsHidden"),
        figureWithheld(review, "handoffs") &&
          t("analytics.team.handoffsHidden"),
      ]
        .filter((note): note is string => typeof note === "string")
        .map((note) => (
          <p key={note} className="t-caption">
            {note}
          </p>
        ))}
    </>
  );
}

/**
 * One row per member, in name order, with the columns the team's workflow
 * asks for. The rows are not a ranking, and a small count is no verdict.
 */
export function MemberTable({
  review,
  zone,
}: Readonly<{ review: Review; zone: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const handoffColumns = useHandoffColumns(review);
  if (review.members_withheld) {
    return <p className="t-caption">{t("analytics.team.membersWithheld")}</p>;
  }
  if (review.members.length === 0) return null;
  const columns =
    review.workflow === "qualification"
      ? qualificationColumns(review, t, locale)
      : handoffColumns;
  const rows = [...review.members].sort((a, b) =>
    forReader(a.label, b.label, locale),
  );
  return (
    <Panel title={t("analytics.team.members")}>
      <DataTable
        label={t("analytics.team.members")}
        rows={rows}
        rowKey={(row) => row.id}
        bleed
        columns={[
          {
            key: "member",
            header: t("analytics.team.member"),
            grow: true,
            render: (row) => row.label,
          },
          ...columns,
        ]}
      />
      <PanelBody>
        <MemberVisibility review={review} />
        {review.workflow === "qualification" && (
          <p className="t-caption">
            {t(
              eventQualifications(review)
                ? "analytics.team.eventCredit"
                : "analytics.team.arrivalCredit",
            )}
          </p>
        )}
        {(review.qualification_unattributed ?? 0) > 0 && (
          <p className="t-caption">
            {plural(
              "analytics.team.qualificationUnattributed",
              review.qualification_unattributed ?? 0,
              {
                count: formatNumber(
                  review.qualification_unattributed ?? 0,
                  locale,
                ),
              },
            )}
          </p>
        )}
        <Disclosure summary={t("reporting.definitions")}>
          {memberNotes(review, zone, t, locale).map((note) => (
            <p key={note} className="t-caption">
              {note}
            </p>
          ))}
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}
