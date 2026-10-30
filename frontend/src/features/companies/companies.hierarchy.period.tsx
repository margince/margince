import { ChevronLeft, ChevronRight } from "lucide-react";
import type { components } from "../../api/schema";
import { IconAction } from "../../design-system/iconaction";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type CompanyHierarchyRollup = components["schemas"]["CompanyHierarchyRollup"];

/** The rollup's period parameters, one of which names the won-sales window. */
export type RollupPeriodField = keyof CompanyHierarchyRollup["current_periods"];

/** A period the reader picked: the parameter and the label the server spelled. */
export type RollupPeriod = Readonly<{
  field: RollupPeriodField;
  label: string;
}>;

const DEFAULT_CHOICE = "quarter_under_way";

const CHOICES: readonly (readonly [string, MessageKey])[] = [
  [DEFAULT_CHOICE, "rollup.period.quarterUnderWay"],
  ["period_year", "rollup.period.year"],
  ["period_quarter", "rollup.period.quarter"],
  ["period_month", "rollup.period.month"],
];

const GRAIN_FIELD: Readonly<
  Record<
    CompanyHierarchyRollup["closed_won_period"]["grain"],
    RollupPeriodField
  >
> = {
  year: "period_year",
  quarter: "period_quarter",
  month: "period_month",
};

function isPeriodField(value: string): value is RollupPeriodField {
  return (
    value === "period_year" ||
    value === "period_quarter" ||
    value === "period_month"
  );
}

/** The query the rollup is read with: no period parameter for the default. */
export function rollupQuery(period: RollupPeriod | null) {
  return period
    ? { scope: "tree" as const, [period.field]: period.label }
    : { scope: "tree" as const };
}

/**
 * Which period the won sales cover, and the steps to its neighbours. Every
 * label comes from the answer, so the screen never spells a fiscal label.
 */
export function RollupPeriodControl({
  rollup,
  period,
  loading,
  onChange,
}: Readonly<{
  rollup: CompanyHierarchyRollup;
  period: RollupPeriod | null;
  /** The answer shown is for the period before the one picked. */
  loading: boolean;
  onChange: (period: RollupPeriod | null) => void;
}>) {
  const t = useT();
  const won = rollup.closed_won_period;
  const field = GRAIN_FIELD[won.grain];
  const next = won.next;
  return (
    <>
      <Select
        aria-label={t("rollup.period.label")}
        options={CHOICES.map(([value, key]) => ({ value, label: t(key) }))}
        value={period?.field ?? DEFAULT_CHOICE}
        onChange={(value) =>
          onChange(
            isPeriodField(value)
              ? { field: value, label: rollup.current_periods[value] }
              : null,
          )
        }
      />
      <IconAction
        label={t("rollup.period.show", { period: won.previous })}
        icon={<ChevronLeft aria-hidden />}
        pending={loading}
        onClick={() => onChange({ field, label: won.previous })}
      />
      <IconAction
        label={
          next
            ? t("rollup.period.show", { period: next })
            : t("rollup.period.nextNotBegun")
        }
        icon={<ChevronRight aria-hidden />}
        pending={loading}
        reason={next ? undefined : t("rollup.period.nextNotBegun")}
        onClick={() => {
          if (next) {
            onChange({ field, label: next });
          }
        }}
      />
    </>
  );
}
