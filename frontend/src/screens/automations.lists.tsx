// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The list pickers a list rule's form draws: the Live List it watches, and
// the Shortlist it adds to, which must hold the same record type. The server
// checks both again on save and at every firing; the picker only keeps the
// reader from choosing a list that could never be accepted.

import type { components } from "../api/schema";
import type { FieldControl } from "../design-system/atoms";
import { Select, type SelectOption } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useLists } from "./lists.queries";
import "./automations.datefield.css";

type ListRow = NonNullable<ReturnType<typeof useLists>["data"]>["data"][number];

/** The lists a picker offers: live ones, and only Shortlists the reader may change. */
function listOptions(
  kind: "live_list" | "shortlist",
  lists: ListRow[] | undefined,
): SelectOption[] {
  return (lists ?? [])
    .filter(
      (list) => !list.archived_at && (kind === "live_list" || list.can_edit),
    )
    .map((list) => ({ value: list.id, label: list.name }));
}

/** Why the picker has nothing to offer, when it has nothing. */
function emptyHint(
  kind: "live_list" | "shortlist",
  failed: boolean,
  empty: boolean,
): MessageKey | undefined {
  if (failed) {
    return "auto.lists.loadError";
  }
  if (!empty) {
    return undefined;
  }
  return kind === "live_list" ? "auto.lists.noLive" : "auto.lists.noShortlist";
}

export function ListParamSelect({
  kind,
  value,
  watchedList,
  onChange,
  control,
}: Readonly<{
  kind: "live_list" | "shortlist";
  value: string;
  /** The Live List the rule watches, whose record type a Shortlist must hold. */
  watchedList: string;
  onChange: (value: string) => void;
  control: FieldControl;
}>) {
  const t = useT();
  const live = useLists({ listType: "dynamic" });
  const watched = live.data?.data.find((list) => list.id === watchedList);
  const needsWatched = kind === "shortlist" && !watched;
  const shortlists = useLists(
    { listType: "static", entityType: watched?.entity_type },
    kind === "shortlist" && !!watched,
  );
  const source = kind === "live_list" ? live : shortlists;
  const options = needsWatched ? [] : listOptions(kind, source.data?.data);
  const hint: MessageKey | undefined = needsWatched
    ? "auto.lists.needsWatched"
    : emptyHint(kind, source.isError, source.isSuccess && options.length === 0);
  return (
    <>
      <Select
        {...control}
        options={options}
        value={value}
        onChange={onChange}
        disabled={options.length === 0}
        placeholder={t("auto.lists.placeholder")}
      />
      {hint && <p className="t-caption param-hint">{t(hint)}</p>}
    </>
  );
}

type Automation = components["schemas"]["Automation"];

/** Why a rule paused itself, in the words its owner resumes it by. */
const PAUSED_REASON: Record<
  NonNullable<Automation["paused_reason"]>,
  MessageKey
> = {
  list_archived: "auto.pausedReason.listArchived",
  list_invalid: "auto.pausedReason.listInvalid",
  list_unavailable: "auto.pausedReason.listUnavailable",
  burst: "auto.pausedReason.burst",
};

/** The reason a paused rule stopped itself; nothing for a rule paused by hand. */
export function RulePausedReason({
  automation,
}: Readonly<{ automation: Automation }>) {
  const t = useT();
  if (automation.status !== "paused" || !automation.paused_reason) {
    return null;
  }
  return (
    <p className="t-caption">{t(PAUSED_REASON[automation.paused_reason])}</p>
  );
}
