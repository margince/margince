import type { components } from "../../api/schema";
import { usePeek } from "../../design-system/peek";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { partialFoot } from "../../lib/standingpeeks";

type ScoreFactor = components["schemas"]["LeadScoreFactor"];

/**
 * The activities behind one behavioral factor, behind the count that says how
 * many there are.
 *
 * The rows read rather than open. Where an activity opens is the timeline's
 * answer, by kind. A second answer here would be one more place to keep in
 * step.
 */
export function FactorSources({ factor }: Readonly<{ factor: ScoreFactor }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();

  // The count is the id array's length, never the named list's. The server
  // omits an id this reader cannot discover, so the list can be shorter. A
  // count that shrank to what one reader may open would tell two readers
  // different things about one score.
  const total = factor.source_activity_ids?.length ?? 0;
  const named = factor.source_activities ?? [];
  const words = t("lead.scoreSources", { count: formatNumber(total, locale) });

  const { ref, props, panel } = usePeek<HTMLButtonElement>({
    content: {
      head: t("lead.scoreSourcesHead"),
      items: named.map((activity) => ({
        key: activity.activity_id,
        // Withheld and absent are different answers. The subject is null for
        // both, told apart by content_state, so the row says which.
        title:
          activity.content_state === "withheld"
            ? t("timeline.withheld")
            : (activity.subject ?? t("lead.scoreSourceNoSubject")),
        sub: formatActivityWhen(
          activity.occurred_at,
          activity.occurred_precision,
          locale,
          zone,
        ),
      })),
      foot: partialFoot(named.length, total, { t, plural, locale, zone }),
    },
    pressOpens: true,
  });

  if (total === 0) {
    return null;
  }
  // A count with no rows behind it stays plain words rather than becoming a
  // door that opens nothing. Two cases reach here: a reader whose scope hides
  // every source, and a server old enough to name none.
  if (named.length === 0) {
    return <>{words}</>;
  }
  return (
    <>
      <button ref={ref} {...props} type="button" className="link-button">
        {words}
      </button>
      {panel}
    </>
  );
}
