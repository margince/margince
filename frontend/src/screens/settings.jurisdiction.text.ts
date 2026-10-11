import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { parsePeriod } from "../format/jurisdiction";
import { type Locale, translatePlural, type useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// How the jurisdiction page says its values: periods in words, classes and
// document labels in the reader's language.

type RetentionClass = components["schemas"]["JurisdictionRetentionClass"];
type Translate = ReturnType<typeof useT>;

export const AUDIENCE_LABEL: Record<
  components["schemas"]["JurisdictionDocument"]["audience"],
  MessageKey
> = {
  employer: "jurisdiction.audience.employer",
  operator: "jurisdiction.audience.operator",
  employee: "jurisdiction.audience.employee",
};

export const DOCUMENT_KIND_LABEL: Record<
  components["schemas"]["JurisdictionDocument"]["kind"],
  MessageKey
> = {
  template: "jurisdiction.documentKind.template",
  reading_copy: "jurisdiction.documentKind.readingCopy",
  record: "jurisdiction.documentKind.record",
};

export function documentFilename(
  document: components["schemas"]["JurisdictionDocument"],
): string {
  return document.media_type === "application/pdf"
    ? `${document.id}.pdf`
    : `${document.id}.md`;
}

const CLASS_LABEL: Record<string, MessageKey> = {
  commercial_correspondence: "jurisdiction.class.commercialCorrespondence",
  accounting_records: "jurisdiction.class.accountingRecords",
};

/** A retention class as a reader says it: "Commercial correspondence, 6 years from the end of the calendar year". */
export function floorText(
  retentionClass: RetentionClass,
  locale: Locale,
  t: Translate,
): string {
  const label = CLASS_LABEL[retentionClass.name];
  const period = periodText(retentionClass.keep, locale);
  return t("jurisdiction.floor.value", {
    class: label ? t(label) : retentionClass.name,
    period: retentionClass.from_year_end
      ? t("jurisdiction.floor.fromYearEnd", { period })
      : period,
  });
}

/** "6 years", "1 year, 6 months"; an unreadable period is shown as written. */
export function periodText(iso: string, locale: Locale): string {
  const period = parsePeriod(iso);
  if (!period) return iso;
  const parts = [
    period.years > 0 &&
      translatePlural(locale, "jurisdiction.years", period.years, {
        count: formatNumber(period.years, locale),
      }),
    period.months > 0 &&
      translatePlural(locale, "jurisdiction.months", period.months, {
        count: formatNumber(period.months, locale),
      }),
    period.days > 0 &&
      translatePlural(locale, "jurisdiction.days", period.days, {
        count: formatNumber(period.days, locale),
      }),
  ].filter((part): part is string => typeof part === "string");
  return parts.join(", ");
}

export function deadlineText(
  deadline: components["schemas"]["JurisdictionDeadline"],
  locale: Locale,
  t: Translate,
): string {
  if (deadline.hours !== undefined) {
    return translatePlural(locale, "jurisdiction.hours", deadline.hours, {
      count: formatNumber(deadline.hours, locale),
    });
  }
  return t("jurisdiction.deadline.fromReceipt", {
    period: periodText(deadline.period ?? "", locale),
  });
}
