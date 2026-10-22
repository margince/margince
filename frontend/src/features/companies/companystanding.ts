import type { components } from "../../api/schema";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  worstOf,
} from "../../components/companylookups";
import {
  type HealthRating,
  type ReadingTone,
  ratingTone,
} from "../../design-system/readingtone";
import type { MessageKey } from "../../i18n/en";

type Moment = components["schemas"]["ContactMoment"];

/** One of the three health dimensions, as the page has rated it. `reason` is
 *  already in the reader's words; absent with no rating. */
export type RatedDimension = Readonly<{
  key: "relationship" | "commercial" | "payment";
  rating?: HealthRating;
  reason?: string;
}>;

export type CompanyVerdict = Readonly<{
  word: MessageKey;
  tone: ReadingTone;
  sentence?: string;
}>;

/**
 * The account's call: the worst rated dimension's word, never an average, and
 * that dimension's reason as the sentence. The Next bar above already says
 * what is owed, so the moment is the sentence only when nothing is rated.
 */
export function companyVerdict({
  dimensions,
  withheld,
  moment,
}: Readonly<{
  dimensions: readonly RatedDimension[];
  // The health section was not shown to this reader, which is not the same
  // fact as nothing rated yet.
  withheld: boolean;
  moment?: Pick<Moment, "rule" | "why_now">;
}>): CompanyVerdict {
  const { overall } = worstOf(dimensions);
  const owed =
    moment && moment.rule !== "nothing_needed" ? moment.why_now : undefined;
  if (!overall) {
    return {
      word: withheld ? "record.notShown" : "standing.notAssessed",
      tone: "neutral",
      sentence: owed,
    };
  }
  const worst = dimensions.find((dimension) => dimension.rating === overall);
  return {
    word: HEALTH_RATING_LABEL[overall],
    tone: ratingTone(overall),
    sentence: worst?.reason ?? owed,
  };
}

export type CompanyDimRow = Readonly<{
  key: RatedDimension["key"];
  label: MessageKey;
  value: MessageKey;
  sub?: string;
  tone: ReadingTone;
}>;

/** The three rows beside the ring, in the ring's own order, so a dot and its
 *  arc always share a colour. */
export function companyDimRows(
  dimensions: readonly RatedDimension[],
  withheld: boolean,
): CompanyDimRow[] {
  return dimensions.map((dimension) => ({
    key: dimension.key,
    label: HEALTH_DIMENSION_LABEL[dimension.key],
    value: dimension.rating
      ? HEALTH_RATING_LABEL[dimension.rating]
      : withheld
        ? "record.notShown"
        : "standing.notAssessed",
    sub: dimension.rating ? dimension.reason : undefined,
    tone: ratingTone(dimension.rating),
  }));
}
