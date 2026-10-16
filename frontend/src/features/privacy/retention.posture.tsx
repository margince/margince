// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { Switch } from "../../design-system/switch";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import {
  RETENTION_POLICIES_KEY,
  RETENTION_SETTINGS_KEY,
} from "./retention.logic";

type RetentionSettings = components["schemas"]["RetentionSettings"];

// Refused, never hidden, without the update grant: every reader needs to know
// which posture is in force, and `reason` says why they cannot change it.
export function PostureToggle({
  retainOnly,
  locks,
  canManage,
}: Readonly<{
  retainOnly: boolean;
  locks: RetentionSettings["locks"];
  canManage: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();

  const update = useMutation({
    mutationFn: async (next: boolean) => {
      return unwrap(
        await api.PATCH("/retention/settings", {
          body: { retain_only: next },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(RETENTION_SETTINGS_KEY, data);
      // Every row's `suppressed_by_posture` derives from the posture just written.
      queryClient.invalidateQueries({ queryKey: RETENTION_POLICIES_KEY });
    },
  });

  return (
    <div className="retention-posture">
      {/* The row draws the naming, so the switch carries the same words hidden. */}
      <Switch
        label={t("retention.retainOnly")}
        labelHidden
        reason={lockReasonOr(
          locks,
          "privacy.retain_only",
          t,
          canManage ? undefined : t("retention.adminOnly"),
        )}
        checked={retainOnly}
        disabled={!canManage}
        pending={update.isPending}
        onChange={(next) => update.mutate(next)}
      />
      <ErrorLine error={update.error} />
    </div>
  );
}
