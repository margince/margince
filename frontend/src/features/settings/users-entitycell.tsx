// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field } from "../../design-system/atoms";
import type { DataTableColumn } from "../../design-system/datatable";
import { InlineChoice } from "../../design-system/inlinechoice";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { type Translator, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import {
  type LegalEntity,
  useAssignLegalEntity,
  useEntityOfUser,
  useLegalEntities,
} from "../../lib/legalentities.queries";
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
  const { entities, fallback } = useEntityChoice(management);
  const membership = useEntityOfUser(fallback ? entities : []);
  // Moving a colleague writes their assignment, which needs both grants.
  const canAssign = useCanWrite("legal_entity", "update") && canChangeRole;
  if (fallback === undefined) {
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

/**
 * The entities a member can be placed in, and the default one a member with
 * no assignment works for. The default is undefined while there is nothing to
 * choose: the reader may not read entities, or the installation has only one.
 */
export function useEntityChoice(enabled: boolean): Readonly<{
  entities: readonly LegalEntity[];
  fallback: LegalEntity | undefined;
}> {
  const shown = useCan("legal_entity", "read") && enabled;
  const entities = useLegalEntities(shown).data ?? [];
  const fallback = entities.find((entity) => entity.is_default);
  if (!shown || entities.length < 2) {
    return { entities: [], fallback: undefined };
  }
  return { entities, fallback };
}

/** Whether the reader may place a member in an entity: both assignment grants. */
export function useMayAssignEntity(): boolean {
  const mayAssignEntity = useCanWrite("legal_entity", "update");
  const mayChangeMembers = useCanWrite("user_admin", "update");
  return mayAssignEntity && mayChangeMembers;
}

/**
 * The edit form's choice of the entity one member works for. current stays
 * undefined for an agent seat and while there is nothing to choose. It also
 * waits for every member list, since a missing assignment cannot be told from
 * one not loaded yet. change is the entity to save, or null.
 */
export function useMemberEntityDraft(member: User) {
  const { entities, fallback } = useEntityChoice(
    useMayAssignEntity() && !member.is_agent,
  );
  const membership = useEntityOfUser(fallback ? entities : []);
  const [chosen, setChosen] = useState<string | null>(null);
  const settled =
    fallback !== undefined && !membership.isPending && !membership.isError;
  const current = settled
    ? (membership.byUser.get(member.id) ?? fallback.id)
    : undefined;
  const change =
    current !== undefined && chosen !== null && chosen !== current
      ? chosen
      : null;
  return {
    entities,
    current,
    value: chosen ?? current,
    change,
    choose: setChosen,
  };
}

export type MemberEntityDraft = ReturnType<typeof useMemberEntityDraft>;

/** The edit form's "Legal entity" field, drawn once there is a choice. */
export function MemberEntityField({
  draft,
}: Readonly<{ draft: MemberEntityDraft }>) {
  const t = useT();
  if (draft.value === undefined) {
    return null;
  }
  return (
    <Field label={t("users.entityLabel")}>
      {(control) => (
        <Select
          {...control}
          value={draft.value ?? ""}
          onChange={draft.choose}
          options={entityOptions(t, draft.entities)}
        />
      )}
    </Field>
  );
}

/** An entity picker's options, each entity under the name it shows. */
export function entityOptions(
  t: Translator,
  entities: readonly LegalEntity[],
): { value: string; label: string }[] {
  return entities.map((entity) => ({
    value: entity.id,
    label: entity.display_name || t("legalEntities.defaultUnnamed"),
  }));
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
  const options = entityOptions(t, entities);
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
        const chosen = options.find((option) => option.value === entityId);
        toast.show(
          t("users.entitySaved", {
            name: member.display_name,
            entity: chosen?.label ?? "",
          }),
        );
      }}
    />
  );
}
