// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { isVersionSkewOf } from "../lib/problem";
import type { useTaskUpdate } from "./taskwrite";

// Drawn outside the verbs. A task changed elsewhere is refetched as done or
// gone, and its verbs vanish; the refusal must outlive them.
export function TaskWriteFailure({
  taskId,
  update,
}: Readonly<{
  taskId: string;
  update: ReturnType<typeof useTaskUpdate>;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  if (!update.isError || update.variables?.id !== taskId) return null;
  return (
    <ErrorLine
      error={update.error}
      actions={
        isVersionSkewOf(update.error) && (
          <Button
            onClick={() => {
              update.reset();
              void queryClient.invalidateQueries({
                queryKey: ["activity", taskId],
              });
            }}
          >
            {t("decision.reRead")}
          </Button>
        )
      }
    />
  );
}
