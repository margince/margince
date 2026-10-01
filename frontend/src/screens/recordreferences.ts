export { searchCompanyTargets as searchCompanyReferences } from "./companyform";
export { searchProjects as searchProjectReferences } from "./companyprojects";

import { useT } from "../i18n";
import { rosterOwnerName, useRoster } from "./entityref";
import { useMemberName } from "./membernames";

export function useRecordOwners(ownerId: string | null | undefined) {
  const t = useT();
  const roster = useRoster("user", true);
  const options = (roster.data ?? []).flatMap((entry) =>
    "display_name" in entry
      ? [{ value: entry.id, label: entry.display_name }]
      : [],
  );
  const missing =
    Boolean(ownerId) && !options.some((entry) => entry.value === ownerId);
  // Named by id: the current owner may be invited or deactivated, and the
  // picker's walk carries neither.
  const ownerName = useMemberName(missing ? ownerId : null);
  if (missing && ownerId)
    options.push({
      value: ownerId,
      label: rosterOwnerName(ownerId, ownerName, t, t("ref.notInRoster")),
    });
  return options;
}
