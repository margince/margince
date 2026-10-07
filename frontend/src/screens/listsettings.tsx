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
import { ListRuleUses, ruleUsesOf } from "./listrules";
import { type List, useUpdateList } from "./lists.queries";
import { type ListAudience, ListAudienceFields } from "./listsharing";

export function ListSettingsAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(list.name);
  const [purpose, setPurpose] = useState(list.purpose ?? "");
  const [audience, setAudience] = useState<ListAudience>(audienceOf(list));
  // What the form opened with, version included. A refetch while it is open
  // must not decide what is sent: sharing and team go only when the reader
  // moved them, so nobody else's change is undone by a save of the name.
  const [started, setStarted] = useState<ListAudience>(audienceOf(list));
  const [version, setVersion] = useState(list.version);
  const me = useMe();
  const update = useUpdateList();
  const sharingChanged = audience.sharing !== started.sharing;
  const teamChanged =
    audience.sharing === "team" && audience.teamId !== started.teamId;
  const openForm = () => {
    setName(list.name);
    setPurpose(list.purpose ?? "");
    setAudience(audienceOf(list));
    setStarted(audienceOf(list));
    setVersion(list.version);
    setOpen(true);
  };
  return (
    <>
      <Button onClick={openForm}>{t("lists.settings")}</Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        intent="form"
        title={t("lists.settingsTitle")}
        confirmLabel={t("lists.save")}
        confirmDisabled={name.trim() === ""}
        pending={update.isPending}
        error={update.isError ? problemMessageOf(update.error, t) : null}
        onConfirm={() =>
          update.mutate(
            {
              id: list.id,
              version,
              name: name.trim(),
              purpose: purpose.trim() === "" ? null : purpose.trim(),
              sharing: sharingChanged ? audience.sharing : undefined,
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
        <ListRuleUses
          rules={ruleUsesOf(list)}
          lead={t(
            list.list_type === "dynamic"
              ? "lists.rules.settingsLeadLive"
              : "lists.rules.settingsLead",
          )}
        />
      </ConfirmModal>
    </>
  );
}

function audienceOf(list: List): ListAudience {
  return { sharing: list.sharing, teamId: list.team_id ?? null };
}
