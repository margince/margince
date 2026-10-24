import type { components } from "../../api/schema";
import { HowItStands } from "../../design-system/howitstands";
import { statTone } from "../../design-system/readingtone";
import { StandingDims } from "../../design-system/standingdims";
import { StrengthRing } from "../../design-system/standingring";
import { StandingTimeline } from "../../design-system/standingtimeline";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatDayMonth } from "../../format/format.dates";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { contactTabRoute } from "../../lib/contacttab";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { activityMarks } from "../../lib/standingmarks";
import { contactDims, contactVerdict } from "./contactstanding";
import { type ContactPeeks, contactPeeks } from "./contactstanding.peeks";
import {
  type ContactFigure,
  contactFigures,
  contactGap,
  contactTimelineEvents,
  type StandingWords,
} from "./contactstandingfigures";
import { changeSentence } from "./relationshipchange";

type Contact360 = components["schemas"]["Contact360"];

/**
 * How the contact stands: the verdict over the strength ring and three
 * readings, the dated timeline, and the four figures under it. Every number
 * is the server's or counted from rows on this read; nothing is a model's.
 */
export function ContactHowItStands({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const words: StandingWords = { t, locale, zone };
  const openHistory = () =>
    navigate(contactTabRoute(view.contact.id, "timeline"));
  const peeks = contactPeeks(view, { t, plural, locale, zone }, openHistory);
  const verdict = contactVerdict(view, t) ?? {
    word: t("record.notShown"),
    tone: "neutral" as const,
  };
  const dims = contactDims(view, t, locale);
  return (
    <HowItStands
      date={formatDayMonth(view.as_of, locale, zone)}
      verdict={verdict}
      hero={<StrengthHero view={view} sentence={verdict.sentence} />}
      dims={
        dims.length > 0 ? (
          <StandingDims
            label={t("contact.standing.dims")}
            dims={dims.map((dim) => ({
              ...dim,
              peek: peeks[dim.key as keyof ContactPeeks],
            }))}
          />
        ) : undefined
      }
      timeline={
        <StandingTimeline
          label={t("standing.timeline.label", {
            name: view.contact.full_name,
          })}
          asOf={view.as_of}
          gap={contactGap(view, words)}
          events={contactTimelineEvents(view, words)}
          marks={activityMarks(view.activities?.data ?? [], {
            t,
            locale,
            zone,
            asOf: view.as_of,
            onOpen: openHistory,
          })}
        />
      }
      figures={
        <StatStrip density="plate">
          {contactFigures(view, words).map((figure) => (
            <StatCard
              key={figure.key}
              label={figure.label}
              value={figure.value}
              detail={figure.detail}
              tone={statTone(figure.tone)}
              peek={peeks[figure.key as keyof ContactPeeks]}
              onOpen={() => openFigure(view.contact.id, figure)}
            />
          ))}
        </StatStrip>
      }
    />
  );
}

function openFigure(contactId: string, figure: ContactFigure) {
  if ("deal" in figure.opens) {
    navigate({ screen: "deals", id: figure.opens.deal });
  } else {
    navigate(contactTabRoute(contactId, figure.opens.tab));
  }
}

// The caption is the newest change of band, unless the verdict already says it.
function StrengthHero({
  view,
  sentence,
}: Readonly<{ view: Contact360; sentence?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const strength = view.strength;
  if (!strength) {
    return null;
  }
  const change = view.relationship_changes?.[0];
  const caption = change ? changeSentence(change, t) : undefined;
  return (
    <div className="pe-standing-hero">
      <StrengthRing
        score={strength.score}
        band={strength.bucket}
        bandLabel={t(`contact.band.${strength.bucket}`)}
        detail={t("contact.strength.detail", {
          last: strength.last_interaction
            ? formatDayMonth(strength.last_interaction, locale, zone)
            : "",
        })}
        explain={strengthExplanation(strength, t, locale, zone)}
      />
      {caption && caption !== sentence && (
        <p className="pe-standing-caption t-caption">{caption}</p>
      )}
    </div>
  );
}

type Strength = NonNullable<Contact360["strength"]>;

const STALE_BELOW = 1 / 3;

// One sentence on where the relationship stands, from the band and how long
// ago the last contact was. The formula behind the score stays out of the way.
function strengthExplanation(
  strength: Strength,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  const last = strength.last_interaction
    ? formatDayMonth(strength.last_interaction, locale, zone)
    : "";
  if (strength.bucket === "none") {
    return t("standing.ring.noInteractions");
  }
  if (strength.bucket === "weak") {
    return t(
      strength.factors.recency < STALE_BELOW
        ? "contact.strength.sentence.weakStale"
        : "contact.strength.sentence.weakRare",
      { last },
    );
  }
  return t(`contact.strength.sentence.${strength.bucket}`, { last });
}
