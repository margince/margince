export { searchCompanyTargets as searchCompanyReferences } from "./companyform";
export { searchProjects as searchProjectReferences } from "./companyprojects";

import { useT } from "../i18n";
import { rosterMissLabel, useRoster, useRosterPartial } from "./entityref";

export function useRecordOwners(ownerId: string | null | undefined) {
  const t = useT();
  const roster = useRoster("user", true);
  const partial = useRosterPartial("user", true);
  const options = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry
      ? [{ value: entry.id, label: entry.display_name }]
      : [],
  );
  if (ownerId && !options.some((entry) => entry.value === ownerId))
    options.push({
      value: ownerId,
      label: rosterMissLabel(roster, partial, t, t("ref.notInRoster")),
    });
  return options;
}
