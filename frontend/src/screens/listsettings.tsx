// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Changing what a list is: its name, what it is for, and who may find it. The
// change carries the version the reader opened, so it cannot land over one
// they never saw.

import { useState } from "react";
import { Button, Field, Textarea, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf } from "./common";
import { SHARING_LABEL } from "./listlibrary";
import { type List, useUpdateList } from "./lists.queries";

const SHARINGS = ["private", "team", "workspace"] as const;

export function ListSettingsAction({ list }: Readonly<{ list: List }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(list.name);
  const [purpose, setPurpose] = useState(list.purpose ?? "");
  const [sharing, setSharing] = useState<List["sharing"]>(list.sharing);
  const update = useUpdateList();
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
              sharing,
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
        <Field label={t("lists.sharingLabel")} hint={t("lists.sharingHint")}>
          {(control) => (
            <Select
              {...control}
              value={sharing}
              onChange={(next) => setSharing(next as List["sharing"])}
              options={SHARINGS.map((value) => ({
                value,
                label: t(SHARING_LABEL[value]),
              }))}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
