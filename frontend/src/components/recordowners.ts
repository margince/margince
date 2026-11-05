import { useT } from "../i18n";
import { useMemberName } from "../lib/membernames";
import { useMe } from "../lib/session";
import { useRecordOwnerOptions } from "./assigneepicker";
import { rosterOwnerName, useRosterHint, useRosterPartial } from "./entityref";

// Only colleagues the server says the reader may make an owner, so the picker
// never offers somebody the save then refuses. The hint says when the roster
// failed or stopped short; without it a failed walk reads as a workspace of one.
export function useRecordOwners(ownerId: string | null | undefined): {
  options: { value: string; label: string }[];
  hint?: string;
} {
  const t = useT();
  const { options, roster } = useRecordOwnerOptions();
  const hint = useRosterHint(roster, useRosterPartial("user", true));
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
  return { options, hint };
}

// The owners, the reader, and the roster's caveat.
export function useOwnerPicker(ownerId: string | null | undefined): {
  owners: { value: string; label: string }[];
  me: string;
  hint?: string;
} {
  const me = useMe();
  const owners = useRecordOwners(ownerId);
  return {
    owners: owners.options,
    me: me.data?.user.id ?? "",
    hint: owners.hint,
  };
}
