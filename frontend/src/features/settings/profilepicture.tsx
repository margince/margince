// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Avatar } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { ImageSlot } from "../../design-system/imageslot";
import { SettingRow } from "../../design-system/settingrow";
import type { Toast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { postMultipart } from "../../lib/multipart";
import { problemMessageOf, problemStatus, unwrap } from "../../lib/problem";

type User = components["schemas"]["User"];

export const PICTURE_TYPES = ["image/png", "image/jpeg", "image/webp"];

export const PICTURE_MAX_BYTES = 5 * 1024 * 1024;

// The server is the authority on both; this only spares a reader the upload.
export function pictureProblem(file: File): MessageKey | null {
  if (!PICTURE_TYPES.includes(file.type)) {
    return "account.pictureWrongType";
  }
  if (file.size > PICTURE_MAX_BYTES) {
    return "account.pictureTooLarge";
  }
  return null;
}

const REFUSAL_BY_STATUS = new Map<number, MessageKey>([
  [413, "account.pictureTooLarge"],
  [415, "account.pictureWrongType"],
  [501, "account.pictureNoStorage"],
]);

// Written on pick, as a `Switch` writes when turned: a chosen file is not a
// draft the save bar could hold and discard.
function usePictureWrite(toast: Toast) {
  const t = useT();
  const client = useQueryClient();
  const write = useMutation({
    mutationFn: async (file: File | null): Promise<void> => {
      if (file === null) {
        unwrap(await api.DELETE("/me/avatar"));
        return;
      }
      await postMultipart("/v1/me/avatar", { file });
    },
    onSuccess: async (_done, file) => {
      toast.show(
        t(file === null ? "account.pictureRemoved" : "account.pictureSaved"),
      );
      await Promise.all([
        client.invalidateQueries({ queryKey: ["me"] }),
        client.invalidateQueries({ queryKey: ["users"] }),
      ]);
    },
  });
  const verb = write.variables === null ? "remove" : "upload";
  return { write, verb } as const;
}

export function ProfilePictureRow({
  user,
  toast,
}: Readonly<{ user: User; toast: Toast }>) {
  const t = useT();
  const { write, verb } = usePictureWrite(toast);
  const [refused, setRefused] = useState<MessageKey | null>(null);
  const name = user.display_name || user.email || "";
  const pick = (file: File | null) => {
    const problem = file === null ? null : pictureProblem(file);
    setRefused(problem);
    if (problem === null) {
      write.mutate(file);
    } else {
      write.reset();
    }
  };
  const refusal = refusalText(refused, write.error, verb, t);
  return (
    <SettingRow
      label={t("account.pictureLabel")}
      description={t("account.pictureHelp")}
      control={(control) => (
        <div className="form-stack settings-picture">
          <ImageSlot
            shape="round"
            filled={Boolean(user.avatar_url)}
            preview={
              <Avatar
                identity={user.id}
                name={name}
                photo={user.avatar_url}
                size="xl"
              />
            }
            editable
            labelledBy={control["aria-labelledby"]}
            describedBy={control["aria-describedby"]}
            refusalId={refusal ? `${control.id}-refusal` : undefined}
            emptyLabel={t("account.pictureEmpty")}
            accept={PICTURE_TYPES.join(",")}
            words={{
              add: t("account.pictureAdd"),
              replace: t("account.pictureReplace"),
              remove: t("account.pictureRemove"),
            }}
            uploading={write.isPending && verb === "upload"}
            removing={write.isPending && verb === "remove"}
            onPick={pick}
            onRemove={() => pick(null)}
          />
          <ErrorLine id={`${control.id}-refusal`}>{refusal}</ErrorLine>
        </div>
      )}
    />
  );
}

function refusalText(
  refused: MessageKey | null,
  error: Error | null,
  verb: "upload" | "remove",
  t: ReturnType<typeof useT>,
): string | null {
  if (refused !== null) {
    return t(refused);
  }
  if (error === null) {
    return null;
  }
  const known = REFUSAL_BY_STATUS.get(problemStatus(error));
  if (known !== undefined) {
    return t(known);
  }
  const fallback = t(
    verb === "remove"
      ? "account.pictureRemoveFailed"
      : "account.pictureUploadFailed",
  );
  return problemMessageOf(error, t, fallback);
}
