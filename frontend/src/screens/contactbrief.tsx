import { Handshake, Minus, TrendingDown, TrendingUp } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Avatar, Button } from "../design-system/atoms";
import { EmailEntry } from "../design-system/emailentry";
import { EmailReference } from "../design-system/emailreference";
import { Panel, PanelBody } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import {
  formatDate,
  formatDayMonth,
  formatNumber,
  relativeDays,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import {
  colleagueWords,
  directionSentence,
  directionWord,
  overallVerdict,
  trendKind,
  trendSentence,
  trendWord,
} from "./contactrail";
import { interactionIcon, useInteractionLabel } from "./interactionchrome";
import { SentenceList, WrittenBy } from "./record360";

type Contact360 = components["schemas"]["Contact360"];
type ContactBrief = components["schemas"]["ContactBrief"];
type Activity = components["schemas"]["Activity"];
type BriefEvidence = components["schemas"]["CompanyBriefEvidence"];

// --- Relationship brief (§5.6) ---------------------------------------------

export function ContactBriefCard({
  brief,
  loading,
  failed = false,
  onRetry,
  view,
  onOpenEmail,
}: Readonly<{
  brief: ContactBrief | undefined;
  loading: boolean;
  failed?: boolean;
  onRetry?: () => void;
  view: Contact360;
  /**
   * Opens a cited message in the record's email drawer. The page owns the
   * drawer, so it owns the opener; a card that mounted its own would put a
   * second one behind the first.
   */
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Resolved from the timeline the page already read rather than fetched: a
  // chip can never name a record the page beside it is withholding.
  const citedActivities = new Map(
    (view.activities?.data ?? []).map((row) => [row.id, row]),
  );
  const written = brief && brief.sentences.length > 0;
  if (!loading && !written && !failed) {
    return (
      <Panel title={t("contact.overview.about")}>
        <PanelBody>
          <p className="pe-prose t-body">
            {[
              view.contact.full_name,
              view.contact.title,
              view.contact.employer?.company_name,
              view.contact.address?.city,
            ]
              .filter(Boolean)
              .join(" · ")}
          </p>
          <p className="t-sub">{t("contact.overview.profileOnly")}</p>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel
      // The page's ONE indigo card: the agent's reading of the relationship,
      // with its claim (who wrote it, as of when) in the head where a reader
      // meets it first. The work queue above stays neutral and marks each
      // suggested row on its own byline, so "a machine did this" is said once
      // per thing a machine did rather than once per card.
      tone="ai"
      title={t("contact.brief.title")}
      titleAction={
        written ? (
          <>
            {/* When before who: the head ends on the claim, the way the queue
                above it ends on its badge, so the two indigo heads close the
                same way. */}
            <span className="t-caption">
              {t("contact.brief.updatedAt", {
                when: formatDate(brief.generated_at, locale, recordZone),
              })}
            </span>
            <WrittenBy by={brief.generated_by} />
          </>
        ) : undefined
      }
    >
      <PanelBody>
        {loading && (
          <p className="pe-prose t-body">{t("contact.brief.reading")}</p>
        )}
        {failed && (
          <p role="alert">
            {t("contact.overview.briefFailed")}{" "}
            <Button variant="ghost" onClick={onRetry}>
              {t("common.retry")}
            </Button>
          </p>
        )}
        {written && (
          <>
            {/* Judgement first: what the brief ADDS to the cards above it is
                what the agent makes of them. The sources are drawn below BY
                TRANSPORT rather than as citation chips, because a chip cannot
                know whether a cited conversation was mail or a chat message,
                and this card's reader has been told wrong before. */}
            <SentenceList
              sentences={brief.sentences}
              leadWithJudgement
              citations="none"
            />
            {/* The relationship's four readings under the sentence they
                qualify: which way it runs, who on our side carries it, what
                is on the table and what is owed. Inside the brief rather
                than as a row of tiles under it, because each is one fact the
                sentence above already rests on, and four cards for four
                words gave each the weight of a section. */}
            <BriefReadings view={view} onOpenEmail={onOpenEmail} />
            <div className="pe-chiprow">
              <span className="t-caption">{t("contact.brief.sources")}</span>
              {/* One source per distinct record, not per citation: several
                  sentences routinely cite the same thread, and naming it once
                  per mention would repeat it and collide on its key. */}
              {[
                ...new Map(
                  brief.sentences.flatMap((sentence) =>
                    sentence.evidence.map(
                      (cited) =>
                        [
                          `${cited.entity_type}-${cited.entity_id}`,
                          cited,
                        ] as const,
                    ),
                  ),
                ).entries(),
              ].map(([key, cited]) => (
                <SourceChip
                  key={key}
                  cited={cited}
                  activity={citedActivities.get(cited.entity_id)}
                  onOpenEmail={onOpenEmail}
                />
              ))}
            </div>
          </>
        )}
      </PanelBody>
    </Panel>
  );
}

// The message the "last reply" reading was read from: the newest inbound
// mail on the page's own timeline, handed to the cell that draws it as the
// row the timeline draws so a reader can open it from here.
function newestInbound(view: Contact360): Activity | undefined {
  return (view.activities?.data ?? [])
    .filter(
      (row) =>
        row.kind === "email" &&
        row.direction === "inbound" &&
        row.email_summary,
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))[0];
}

// The five cells of the brief's pulse row, built apart from the component
// that draws them so the reading rules and the rendering are two short
// pieces rather than one long one.
function briefCells(
  view: Contact360,
  t: ReturnType<typeof useT>,
  locale: Locale,
  omitted: ReadonlySet<string>,
  lastInbound: Activity | undefined,
  onOpenEmail: ((activityId: string) => void) | undefined,
) {
  const touch = (value: () => string) =>
    omitted.has("last_touch") ? t("record.notShown") : value();
  const overall = omitted.has("last_touch") ? null : overallVerdict(view);
  return [
    {
      key: "overall",
      term: t("contact.rail.overall"),
      value: overall ? overall.word(t) : t("record.notShown"),
      tone: overall?.tone,
    },
    {
      key: "direction",
      term: t("contact.rail.direction"),
      value: touch(() => directionWord(view, t)),
      detail: omitted.has("last_touch") ? null : (
        <p className="t-body pe-brief-gloss">{directionSentence(view, t)}</p>
      ),
    },
    {
      key: "lastReply",
      term: t("contact.rail.lastReply"),
      value: touch(() =>
        relativeDays(view.last_inbound_at, t, locale, new Date(view.as_of)),
      ),
      detail:
        !omitted.has("last_touch") && lastInbound?.email_summary ? (
          <EmailEntry
            summary={lastInbound.email_summary}
            onOpen={
              onOpenEmail &&
              lastInbound.email_summary.display_status !== "withheld"
                ? () => onOpenEmail(lastInbound.id)
                : undefined
            }
            whyNotOpenable="noDetail"
          />
        ) : null,
    },
    {
      key: "coverage",
      term: t("contact.rail.coverage"),
      value: omitted.has("network")
        ? t("record.notShown")
        : colleagueWords(view.network?.colleagues?.length ?? 0, locale),
      // WHO the colleagues are, behind the count: a reader who sees "1
      // colleague" wants the name, and the proof beside it (exchanges in the
      // last ninety days, never a ranking nobody can check).
      detail: omitted.has("network") ? null : <Colleagues view={view} />,
    },
    {
      key: "trend",
      term: t("contact.rail.trend"),
      value: touch(() => trendWord(view, t)),
      // The direction as a glyph before the word, so the row reads at a
      // glance: up for warming, down for cooling, a dash when nothing has
      // come in to read a direction from.
      glyph: omitted.has("last_touch") ? null : <TrendGlyph view={view} />,
      detail:
        omitted.has("last_touch") || trendKind(view) === "none" ? null : (
          <p className="t-body pe-brief-gloss">{trendSentence(view, t)}</p>
        ),
    },
  ];
}

// The relationship's pulse, as a row of caption-over-value cells under the
// sentence it qualifies: which way it runs, when they last wrote, who on our
// side carries it, where it is heading, and the verdict those four add up to.
// Four of the five are read from the two touch dates, which one grant
// governs, so one withheld section blanks them together; coverage is the
// network's, on its own grant.
function BriefReadings({
  view,
  onOpenEmail,
}: Readonly<{
  view: Contact360;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const omitted = new Set(view.sections_omitted ?? []);
  const cells = briefCells(
    view,
    t,
    locale,
    omitted,
    newestInbound(view),
    onOpenEmail,
  );
  return (
    <dl className="pe-brief-readings">
      {cells.map((cell) => (
        <div key={cell.key} className="pe-brief-reading">
          <dt className="t-caption">{cell.term}</dt>
          <dd
            className={
              [
                cell.key === "overall" ? "pe-brief-reading-verdict" : "",
                cell.tone ? `pe-brief-reading-${cell.tone}` : "",
              ]
                .filter(Boolean)
                .join(" ") || undefined
            }
          >
            {"glyph" in cell && cell.glyph}
            {"detail" in cell && cell.detail ? (
              <Popover onHover label={cell.value}>
                {cell.detail}
              </Popover>
            ) : (
              cell.value
            )}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// The trend's glyph in the trend's tone.
function TrendGlyph({ view }: Readonly<{ view: Contact360 }>): ReactNode {
  const kind = trendKind(view);
  const Glyph =
    kind === "warming" ? TrendingUp : kind === "cooling" ? TrendingDown : Minus;
  return (
    <Glyph
      size={15}
      aria-hidden="true"
      className={`pe-brief-trend pe-brief-trend-${kind}`}
    />
  );
}

// The colleagues who also know this contact, with the proof beside each:
// exchanges in the last ninety days, never a ranking nobody can check.
function Colleagues({ view }: Readonly<{ view: Contact360 }>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const colleagues = view.network?.colleagues ?? [];
  if (colleagues.length === 0) {
    return null;
  }
  return (
    <ul className="pe-colleagues">
      {colleagues.map((colleague) => (
        <li key={colleague.user_id} className="pe-colleague">
          <Avatar name={colleague.display_name} />
          <span>
            <span className="pe-colleague-name">{colleague.display_name}</span>
            <span className="pe-colleague-proof t-caption">
              {t("contact.rail.exchanges", {
                count: formatNumber(colleague.interactions_90d, locale),
              })}
            </span>
          </span>
        </li>
      ))}
    </ul>
  );
}

// One cited source, named for what it is.
//
// A citation carries a RECORD TYPE and an id, never a transport — so an
// activity citation says nothing at all about how the conversation was
// carried, and calling every one of them a mail thread told the reader of a
// contact with no email address that they had been mailed. The activity the
// page ALREADY read is the resolver: it carries the kind, and for a message
// the provider the directory names. A citation whose activity is not on this
// page is named for the one thing it certainly is — a conversation — rather
// than for a transport nobody checked.
function SourceChip({
  cited,
  activity,
  onOpenEmail,
}: Readonly<{
  cited: BriefEvidence;
  activity: Activity | undefined;
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const interactionLabel = useInteractionLabel();
  if (cited.entity_type === "activity") {
    // A cited EMAIL is named by its subject and openable, like every other
    // citation of a message in the product: as a plain link in the sources
    // line, because a source is a footnote and not a status. A reference
    // reading "Email" would tell a reader which transport carried the sentence
    // and nothing about which message; the one they want is the one the
    // sentence rests on.
    //
    // `email_summary` is the server's own answer to "is this an email", set
    // only for kind=email, so nothing here decides it from the kind string. A
    // withheld message carries the summary with its subject nulled, and the
    // line says so and opens nothing.
    const summary = activity?.email_summary;
    if (summary) {
      return (
        <EmailReference
          subject={summary.subject}
          withheld={summary.display_status === "withheld"}
          onOpen={
            onOpenEmail ? () => onOpenEmail(summary.activity_id) : undefined
          }
        />
      );
    }
    // A meeting, a call, a chat: the transport's own name and the day, as
    // words, so "Meeting 17 Sept" reads as the record it is.
    // The glyph says what KIND of record the source is, the way the
    // timeline marks its rows, so a meeting and a call read apart before
    // their words do.
    return (
      <span className="t-sub pe-source">
        {activity && interactionIcon(activity.kind)}
        {activity
          ? `${interactionLabel(activity.kind, activity.channel_provider)} ${formatDayMonth(activity.occurred_at, locale, recordZone)}`
          : t("contact.brief.sourceActivity")}
      </span>
    );
  }
  return (
    <span className="t-sub pe-source">
      {cited.entity_type === "deal" && (
        <Handshake size={13} aria-hidden="true" />
      )}
      {cited.entity_type === "deal"
        ? t("contact.brief.sourceDeal")
        : cited.entity_type}
    </span>
  );
}
