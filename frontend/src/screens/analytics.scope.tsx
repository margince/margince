import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { AnalyticsScope } from "./analytics.context";
import { scopeKey } from "./analytics.context";

/**
 * Which population the numbers on this page are about.
 *
 * The options come from the server, labels included. Nothing here decides who
 * may measure what: a control offering only permitted populations is a
 * convenience, and every request is validated again on the way in.
 *
 * A caller with exactly one population to measure — a rep — gets a plain line
 * rather than a control, because a dropdown holding one option asks a question
 * with one answer.
 */
export function AnalyticsScopePicker({
  scopes,
  selected,
  onSelect,
}: Readonly<{
  scopes: readonly AnalyticsScope[];
  selected: AnalyticsScope;
  onSelect: (scope: AnalyticsScope) => void;
}>) {
  const t = useT();

  if (scopes.length <= 1) {
    return (
      <p className="sub">
        {t("analytics.scopeFixed", { scope: selected.label })}
      </p>
    );
  }

  return (
    <Select
      aria-label={t("analytics.scopeLabel")}
      options={scopes.map((scope) => ({
        value: scopeKey(scope),
        label: scope.label,
      }))}
      value={scopeKey(selected)}
      onChange={(next) => {
        const picked = scopes.find((scope) => scopeKey(scope) === next);
        // A value that matches no option cannot come from this list, so it is
        // ignored rather than guessed at: guessing would measure a population
        // the reader did not choose and label it with one they did.
        if (picked) {
          onSelect(picked);
        }
      }}
    />
  );
}
