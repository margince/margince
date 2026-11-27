// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Changing what a list is: its name, what it is for, and who may find it. The
// change carries the version the reader opened, so it cannot land over one
// they never saw.

import { useState } from "react";
import { Button, Field, Textarea, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { useToast } from "../../design-system/toast";
import { useLocale, useT } from "../../i18n";
import { type List, useUpdateList } from "../../lib/lists.queries";
import { blankRequired, refusalSentences } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { ownedByReader } from "./library";
import { ListRuleUses, ruleUsesOf } from "./listrules";
import { type ListAudience, ListAudienceFields } from "./listsharing";

export function ListSettingsAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const toast = useToast();
  const nameLabel = t("lists.name");
  const purposeLabel = t("lists.purpose");
  // A refused name or purpose is said on its field, which the sentence names.
  const labels = { name: nameLabel, purpose: purposeLabel };
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
    faults.show(new Map());
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
        bodyRef={faultsRoot}
        pending={update.isPending}
        error={refusalSentences(update.error, t, labels).rest}
        onConfirm={() => {
          if (faults.show(blankRequired(locale, { name: [nameLabel, name] }))) {
            return;
          }
          update.mutate(
            {
              id: list.id,
              version,
              name: name.trim(),
              purpose: purpose.trim() === "" ? null : purpose.trim(),
              sharing: sharingChanged ? audience.sharing : undefined,
              teamId: teamChanged ? audience.teamId : undefined,
            },
            {
              onSuccess: () => {
                setOpen(false);
                toast.show(t("lists.settingsSaved"));
              },
              onError: (error) =>
                faults.show(refusalSentences(error, t, labels).byField),
            },
          );
        }}
      >
        <Field label={nameLabel} required error={faults.of("name")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => {
                setName(event.target.value);
                faults.edited("name");
              }}
            />
          )}
        </Field>
        <Field label={purposeLabel} error={faults.of("purpose")}>
          {(control) => (
            <Textarea
              {...control}
              value={purpose}
              onChange={(event) => {
                setPurpose(event.target.value);
                faults.edited("purpose");
              }}
            />
          )}
        </Field>
        <ListAudienceFields
          value={audience}
          onChange={setAudience}
          ownerIsReader={ownedByReader(list, me.data?.user.id)}
          openedWith={started.sharing}
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
