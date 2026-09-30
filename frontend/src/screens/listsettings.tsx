// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Changing what a list is: its name, what it is for, and who may find it. The
// change carries the version the reader opened, so it cannot land over one
// they never saw.

import { useState } from "react";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, useMe } from "./common";
import { type List, useUpdateList } from "./lists.queries";
import { type ListAudience, ListAudienceFields } from "./listsharing";

export function ListSettingsAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(list.name);
  const [purpose, setPurpose] = useState(list.purpose ?? "");
  const [audience, setAudience] = useState<ListAudience>({
    sharing: list.sharing,
    teamId: list.team_id ?? null,
  });
  const me = useMe();
  const update = useUpdateList();
  // The team travels only when it changed under team sharing: an untouched
  // one would write a revision that changes nothing.
  const currentTeam = list.team_id ?? null;
  const teamChanged =
    audience.sharing === "team" && audience.teamId !== currentTeam;
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lists.settings")}</Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("lists.settingsTitle")}
        confirmLabel={t("lists.save")}
        confirmDisabled={name.trim() === ""}
        pending={update.isPending}
        error={update.isError ? problemMessageOf(update.error, t) : null}
        onConfirm={() =>
          update.mutate(
            {
              id: list.id,
              version: list.version,
              name: name.trim(),
              purpose: purpose.trim() === "" ? null : purpose.trim(),
              sharing: audience.sharing,
              teamId: teamChanged ? audience.teamId : undefined,
            },
            { onSuccess: () => setOpen(false) },
          )
        }
      >
        <Field label={t("lists.name")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("lists.purpose")}>
          {(control) => (
            <Textarea
              {...control}
              value={purpose}
              onChange={(event) => setPurpose(event.target.value)}
            />
          )}
        </Field>
        <ListAudienceFields
          value={audience}
          onChange={setAudience}
          ownerIsReader={list.owner_id === me.data?.user.id}
        />
      </ConfirmModal>
    </>
  );
}
