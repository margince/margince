// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { isMessageKey, useT } from "../i18n";
import { Select, type SelectOption } from "./select";

// Shared between the tool console's passport filter and the OAuth consent
// screen — extracted so the two surfaces cannot drift into rendering "which
// passport, which scopes" differently. A caller supplies
// its own already-localized `label` per option (e.g. the tool console's
// "Reachable by {name}" phrasing); this component only lays the list out.
export type PassportOption = {
  id: string;
  label: string;
  scopes: string[];
};

// scopeChipLabel resolves one scope token to the words a reader sees,
// falling back to the raw token when the catalogue has no entry — an
// unrecognized chip is a sign the vocabulary grew, not a reason to hide
// what was granted. Exported so every caller of ScopeChips translates the
// same way, rather than each re-deriving the fallback rule: the design
// system lays chips out, but copy is authored where every other label on
// these screens is (PassportOption.label above is the same convention).
export function scopeChipLabel(
  t: ReturnType<typeof useT>,
  scope: string,
): string {
  const key = `passport.scope.${scope}`;
  return isMessageKey(key) ? t(key) : scope;
}

// A scope chip row: every chip a scope the passport carries, already
// resolved to its display label by the caller. There is no "granted versus
// not" distinction to draw — the tool console lists a whole passport's
// scopes and a connection's own summary is scoped the same way — so a chip
// means one thing on both surfaces.
export function ScopeChips({ labels }: Readonly<{ labels: string[] }>) {
  return (
    <>
      {labels.map((label) => (
        <span key={label} className="badge">
          {label}
        </span>
      ))}
    </>
  );
}

export function PassportSelect({
  options,
  value,
  onChange,
  allowEmpty,
  emptyLabel,
  ariaLabel,
}: Readonly<{
  options: readonly PassportOption[];
  value: string;
  onChange: (id: string) => void;
  // The tool console offers an "all passports" choice (no passport picked
  // means every tool row reads as reachable); the consent screen always
  // requires one, so it leaves this unset.
  allowEmpty?: boolean;
  emptyLabel?: string;
  // Falls back to the generic catalog label; the tool console passes its own
  // ("All passports") so its existing accessible name doesn't move.
  ariaLabel?: string;
}>) {
  const t = useT();
  // The empty choice is an OPTION, not the select's placeholder: a placeholder
  // is a face, and the tool console's reader has to be able to come back to
  // "all passports" after picking one.
  const empty: SelectOption[] = allowEmpty
    ? [{ value: "", label: emptyLabel ?? t("passport.noneOption") }]
    : [];
  return (
    <Select
      aria-label={ariaLabel ?? t("passport.select")}
      options={[
        ...empty,
        ...options.map((option) => ({
          value: option.id,
          label: option.label,
        })),
      ]}
      value={value}
      onChange={onChange}
    />
  );
}
