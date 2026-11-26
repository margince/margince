// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";

// The reader-team controls of a mailbox-like seat. A shared mailbox and a
// Telegram Business account both pick the team that reads them. A `held`
// message stays with that team, and a `shared` one follows its record.

type Team = components["schemas"]["Team"];
export type Visibility =
  components["schemas"]["SharedMailbox"]["mail_visibility"];

// The team roster's entries, typed as teams: the shared roster cache holds
// seats and teams under one entry type.
export function isTeam(entry: { id: string }): entry is Team {
  return "name" in entry;
}

export const VISIBILITY_LABEL: Record<Visibility, MessageKey> = {
  held: "sharedMailboxes.visibilityHeld",
  shared: "sharedMailboxes.visibilityShared",
};

const VISIBILITIES: readonly Visibility[] = ["held", "shared"];

const MAILBOX_HINTS: Record<Visibility, MessageKey> = {
  held: "sharedMailboxes.visibilityHeldHint",
  shared: "sharedMailboxes.visibilitySharedHint",
};

export function TeamPicker({
  teams,
  value,
  onChange,
  error,
}: Readonly<{
  teams: readonly Team[];
  value: string;
  onChange: (id: string) => void;
  error?: string;
}>) {
  const t = useT();
  return (
    <Field label={t("sharedMailboxes.readerTeam")} required error={error}>
      {(control) => (
        <Select
          {...control}
          value={value}
          onChange={onChange}
          placeholder={t("sharedMailboxes.readerTeamPlaceholder")}
          options={teams.map((team) => ({ value: team.id, label: team.name }))}
        />
      )}
    </Field>
  );
}

/** `hints` replaces the mailbox wording where Shared follows another rule. */
export function VisibilityChoice({
  value,
  onChange,
  hints = MAILBOX_HINTS,
}: Readonly<{
  value: Visibility;
  onChange: (next: Visibility) => void;
  hints?: Record<Visibility, MessageKey>;
}>) {
  const t = useT();
  return (
    <ChoiceList
      legend={t("sharedMailboxes.visibility")}
      value={value}
      onChange={onChange}
      choices={VISIBILITIES.map((choice) => ({
        value: choice,
        label: t(labelFor(VISIBILITY_LABEL, choice)),
        description: t(labelFor(hints, choice)),
      }))}
    />
  );
}
