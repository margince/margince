// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A preview cell that holds an id pointing at another record, named the way the
// rest of the product names that record: a colleague by id, a team off the
// roster walk, a company or project off its own read, and workspace
// configuration off the filter builder's own option lists. The id itself is
// never the label, because a reader checking a filter cannot act on a UUID.

import {
  missLabel,
  rosterMissLabel,
  useEntityName,
  useRoster,
  useRosterPartial,
} from "../../components/entityref";
import { readingOf, usableName } from "../../components/entityref.queries";
import { useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { type Reference, useReferenceOptions } from "./filterreference";

type CellProps = Readonly<{ id: string }>;

export function ReferenceCell({
  reference,
  id,
}: Readonly<{ reference: Reference; id: string }>) {
  switch (reference) {
    case "app_user":
      return <UserCell id={id} />;
    case "team":
      return <TeamCell id={id} />;
    case "company":
    case "project":
      return <RecordCell kind={reference} id={id} />;
    default:
      return <OptionCell reference={reference} id={id} />;
  }
}

function UserCell({ id }: CellProps) {
  const t = useT();
  const query = useMemberName(id);
  const name = usableName(query.data);
  return (
    <span title={id}>
      {name ?? missLabel(readingOf(query), t, t("filters.resultRefGone"))}
    </span>
  );
}

function TeamCell({ id }: CellProps) {
  const t = useT();
  const roster = useRoster("team", true);
  const partial = useRosterPartial("team", true);
  const match = roster.data?.find((entry) => entry.id === id);
  const name = match && "name" in match ? usableName(match.name) : null;
  return (
    <span title={id}>
      {name ?? rosterMissLabel(roster, partial, t, t("filters.resultRefGone"))}
    </span>
  );
}

function RecordCell({
  kind,
  id,
}: Readonly<{ kind: "company" | "project"; id: string }>) {
  const t = useT();
  const { name, reading } = useEntityName(kind, id);
  return (
    <span title={id}>
      {name ?? missLabel(reading, t, t("filters.resultRefGone"))}
    </span>
  );
}

function OptionCell({
  reference,
  id,
}: Readonly<{ reference: Reference; id: string }>) {
  const t = useT();
  const { options, loading, failed } = useReferenceOptions(reference);
  const label = options.find((option) => option.value === id)?.label;
  const reading = readingOf({ isPending: loading, isError: failed });
  return (
    <span title={id}>
      {usableName(label) ?? missLabel(reading, t, t("filters.resultRefGone"))}
    </span>
  );
}
