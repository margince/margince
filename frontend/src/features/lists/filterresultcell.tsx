// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A preview cell whose value is the id of another record, shown by name. Each
// kind of record is named by the read the rest of the product uses for it. The
// id is never the label, because a reader cannot act on a UUID.

import type { CompanyMarksByID } from "../../components/companymarks";
import {
  missLabel,
  rosterMissLabel,
  useEntityName,
  useRoster,
  useRosterPartial,
} from "../../components/entityref";
import {
  type NameReading,
  readingOf,
  usableName,
} from "../../components/entityref.queries";
import {
  type Reference,
  useReferenceOptions,
} from "../../components/filterreference";
import { useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";

type CellProps = Readonly<{ id: string }>;

export function ReferenceCell({
  reference,
  id,
  companies,
}: Readonly<{
  reference: Reference;
  id: string;
  /** The page's companies, read once by the table rather than per cell. */
  companies: CompanyMarksByID;
}>) {
  switch (reference) {
    case "app_user":
      return <UserCell id={id} />;
    case "team":
      return <TeamCell id={id} />;
    case "company":
      return <CompanyCell id={id} companies={companies} />;
    case "project":
      return <ProjectCell id={id} />;
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

function CompanyCell({
  id,
  companies,
}: Readonly<{ id: string; companies: CompanyMarksByID }>) {
  const t = useT();
  const name = usableName(companies.marks.get(id)?.name);
  const reading: NameReading = companies.pending.has(id)
    ? "pending"
    : companies.unreadable.has(id)
      ? "failed"
      : "unnamed";
  return (
    <span title={id}>
      {name ?? missLabel(reading, t, t("filters.resultRefGone"))}
    </span>
  );
}

// `/projects` takes no id filter, so each project is named by its own read.
// The query is keyed by id, so a project named on many rows is read once.
function ProjectCell({ id }: CellProps) {
  const t = useT();
  const { name, reading } = useEntityName("project", id);
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
