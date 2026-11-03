// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { DataTableColumn } from "../../design-system/datatable";
import { InlineChoice } from "../../design-system/inlinechoice";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import {
  type LegalEntity,
  useAssignLegalEntity,
  useEntityOfUser,
  useLegalEntities,
} from "./legalentities.queries";
import type { User } from "./users-members";

/**
 * The roster's "Legal entity" column, or null while the installation trades
 * as one entity. With nothing to choose between, the column would only repeat
 * the company's name on every row.
 */
export function useEntityColumn(
  management: boolean,
  canChangeRole: boolean,
): DataTableColumn<User> | null {
  const t = useT();
  const mayRead = useCan("legal_entity", "read");
  const shown = management && mayRead;
  const entities = useLegalEntities(shown).data ?? [];
  const membership = useEntityOfUser(shown ? entities : []);
  // Moving a colleague writes their assignment, which needs both grants.
  const canAssign = useCanWrite("legal_entity", "update") && canChangeRole;
  const fallback = entities.find((entity) => entity.is_default);
  if (!shown || entities.length < 2 || fallback === undefined) {
    return null;
  }
  return {
    key: "entity",
    header: t("users.entityLabel"),
    render: (member) =>
      // Until every list has answered, a missing assignment cannot be told
      // from one not loaded yet, and the cell would name the default wrongly.
      membership.isPending || membership.isError || member.is_agent ? null : (
        <MemberEntity
          member={member}
          entities={entities}
          current={membership.byUser.get(member.id) ?? fallback.id}
          canAssign={canAssign}
        />
      ),
  };
}

export function MemberEntity({
  member,
  entities,
  current,
  canAssign,
}: Readonly<{
  member: User;
  entities: readonly LegalEntity[];
  current: string;
  canAssign: boolean;
}>) {
  const t = useT();
  const toast = useToast();
  const assign = useAssignLegalEntity();
  const nameOf = (entity: LegalEntity) =>
    entity.display_name || t("legalEntities.defaultUnnamed");
  const options = entities.map((entity) => ({
    value: entity.id,
    label: nameOf(entity),
  }));
  return (
    <InlineChoice
      label={t("users.entityOf", { name: member.display_name })}
      hideLabel
      value={current}
      options={options}
      canEdit={canAssign}
      render={(value) =>
        options.find((option) => option.value === value)?.label ?? ""
      }
      onSave={async (entityId) => {
        await assign.mutateAsync({ entityId, userId: member.id });
        const chosen = entities.find((entity) => entity.id === entityId);
        toast.show(
          t("users.entitySaved", {
            name: member.display_name,
            entity: chosen ? nameOf(chosen) : "",
          }),
        );
      }}
    />
  );
}
