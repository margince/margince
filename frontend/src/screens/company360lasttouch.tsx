import type { components } from "../api/schema";
import { StatCard } from "../design-system/atoms";
import { formatDateAbbrev, formatNumber } from "../format/format";
import type { Locale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { daysAgo, WITHHELD_READING } from "./company360health";

type Company360 = components["schemas"]["Company360"];

// The last contact with the account, as days since it happened and what it
// was. The server picks it (`last_contact`), so this tile and the timeline's
// last-contact stop name the same activity.
export function LastTouchStat({
  view,
  withheld,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  view?: Company360;
  // Refused, read once by the strip and shared with the relationship card.
  withheld: boolean;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const slot = { label: t("co.strip.lastTouch"), narrow: "row" } as const;
  if (!view || withheld) {
    return <StatCard onOpen={onOpen} {...slot} value={t(WITHHELD_READING)} />;
  }
  const last = view.last_contact;
  if (!last) {
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t("co.strip.lastTouch.never")}
      />
    );
  }
  const days = daysAgo(last.at, view.as_of);
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={
        days === undefined
          ? t("co.strip.lastTouch.today")
          : t("co.strip.lastTouch.ago", { count: formatNumber(days, locale) })
      }
      detail={`${t(LAST_CONTACT_KIND[last.kind])} · ${formatDateAbbrev(last.at, locale, recordZone)}`}
    />
  );
}

const LAST_CONTACT_KIND = {
  email: "co.spine.kind.email",
  call: "co.spine.kind.call",
  meeting: "co.spine.kind.meeting",
  message: "co.spine.kind.message",
} as const satisfies Record<
  NonNullable<Company360["last_contact"]>["kind"],
  MessageKey
>;
