export { searchProjects as searchProjectReferences } from "../companies/companyprojects";

import { useRecordOwnerOptions } from "../../components/assigneepicker";
import {
  rosterOwnerName,
  useRoster,
  useRosterPartial,
  useRosterPartialHint,
} from "../../components/entityref";
import { useT } from "../../i18n";
import { useMemberName } from "../../lib/membernames";
import { useMe } from "../../lib/session";

// Only colleagues the server says the reader may make an owner, so the picker
// never offers somebody the save then refuses.
export function useRecordOwners(ownerId: string | null | undefined) {
  const t = useT();
  const options = useRecordOwnerOptions();
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

// The owners, the reader, and the caveat a picker owes when the roster
// failed or stopped short. Without the caveat a failed walk reads as a
// workspace of one.
export function useOwnerPicker(ownerId: string | null | undefined): {
  owners: { value: string; label: string }[];
  me: string;
  hint?: string;
} {
  const t = useT();
  const me = useMe();
  const owners = useRecordOwners(ownerId);
  const roster = useRoster("user", true);
  const partialHint = useRosterPartialHint(useRosterPartial("user", true));
  return {
    owners,
    me: me.data?.user.id ?? "",
    hint: roster.isError ? t("owner.rosterFailed") : partialHint,
  };
}
