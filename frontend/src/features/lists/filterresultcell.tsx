// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A preview cell whose value is the id of another record, shown by name. Each
// kind of record is named by the read the rest of the product uses for it. The
// id is never the label, because a reader cannot act on a UUID.

import type { CompanyMarksByID } from "../../components/companymarks";
import {
  missLabel,
  rosterReading,
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

// The id is the tooltip only where no name stands in for it, as `EntityRef` does.
function Named({
  id,
  name,
  reading,
}: Readonly<{ id: string; name: string | null; reading: NameReading }>) {
  const t = useT();
  if (name) {
    return <span>{name}</span>;
  }
  const miss = missLabel(reading, t, t("filters.resultRefGone"));
  return reading === "pending" ? (
    <span>{miss}</span>
  ) : (
    <span title={id}>{miss}</span>
  );
}

function UserCell({ id }: CellProps) {
  const query = useMemberName(id);
  return (
    <Named id={id} name={usableName(query.data)} reading={readingOf(query)} />
  );
}

function TeamCell({ id }: CellProps) {
  const roster = useRoster("team", true);
  const partial = useRosterPartial("team", true);
  const match = roster.data?.find((entry) => entry.id === id);
  const name = match && "name" in match ? usableName(match.name) : null;
  return <Named id={id} name={name} reading={rosterReading(roster, partial)} />;
}

function CompanyCell({
  id,
  companies,
}: Readonly<{ id: string; companies: CompanyMarksByID }>) {
  const reading: NameReading = companies.pending.has(id)
    ? "pending"
    : companies.unreadable.has(id)
      ? "failed"
      : "unnamed";
  return (
    <Named
      id={id}
      name={usableName(companies.marks.get(id)?.name)}
      reading={reading}
    />
  );
}

// `/projects` takes no id filter, so each project is named by its own read.
// The query is keyed by id, so a project named on many rows is read once.
function ProjectCell({ id }: CellProps) {
  const { name, reading } = useEntityName("project", id);
  return <Named id={id} name={name} reading={reading} />;
}

function OptionCell({
  reference,
  id,
}: Readonly<{ reference: Reference; id: string }>) {
  const { options, loading, failed } = useReferenceOptions(reference);
  const label = options.find((option) => option.value === id)?.label;
  return (
    <Named
      id={id}
      name={usableName(label)}
      reading={readingOf({ isPending: loading, isError: failed })}
    />
  );
}
