// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { ANALYTICS_CONTEXT_KEY } from "../../lib/analyticskeys";
import { problemMessageOf, unwrap } from "../../lib/problem";

/** The team a lifecycle write names; the name rides along for its toast. */
export type TeamRef = Readonly<{ id: string; name: string }>;
type ArchivableTeam = TeamRef & Readonly<{ member_count?: number }>;

async function setArchived(id: string, archived: boolean) {
  unwrap(
    await api.PATCH("/teams/{id}", {
      params: { path: { id } },
      body: { archived },
    }),
  );
}

// Archive and restore of one team. The archive's Undo and the Restore on an
// archived row are the same write, so a refusal reads the same from both.
export function useTeamLifecycle() {
  const t = useT();
  const toast = useToast();
  const qc = useQueryClient();
  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["teams"] });
    qc.invalidateQueries({ queryKey: ANALYTICS_CONTEXT_KEY });
  };
  const restore = useMutation({
    mutationFn: ({ id }: TeamRef) => setArchived(id, false),
    // Sticky: an Undo is consumed by the press, so a quiet refusal would read
    // as the team having come back.
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true });
    },
    onSuccess: (_restored, team) => {
      refresh();
      toast.show(t("users.teamRestored", { name: team.name }));
    },
  });
  const archive = useMutation({
    mutationFn: ({ id }: TeamRef) => setArchived(id, true),
    onSuccess: (_archived, team) => {
      refresh();
      toast.show(t("users.teamArchived", { name: team.name }), {
        action: {
          kind: "undo",
          label: t("common.undo"),
          onAct: () => restore.mutate(team),
        },
      });
    },
  });
  return { archive, restore };
}

// The archive verb as the Teams page offers it. A team with members asks
// first; an empty one is archived at once, with Undo.
export function useArchiveFlow() {
  const { archive, restore } = useTeamLifecycle();
  const [confirming, setConfirming] = useState<ArchivableTeam | null>(null);
  const ask = (team: ArchivableTeam) => {
    if ((team.member_count ?? 0) > 0) {
      archive.reset();
      setConfirming(team);
      return;
    }
    archive.mutate(team);
  };
  const confirm = {
    team: confirming,
    members: confirming?.member_count ?? 0,
    pending: archive.isPending,
    error: archive.error,
    onConfirm: (team: TeamRef) =>
      archive.mutate(team, { onSuccess: () => setConfirming(null) }),
    onClose: () => {
      archive.reset();
      setConfirming(null);
    },
  };
  return { archive, restore, ask, confirming: confirming !== null, confirm };
}

// Archiving takes the team's reach away from every member at once, so a team
// with members asks first and says how many lose it.
export function ArchiveTeamConfirm({
  team,
  members,
  pending,
  error,
  onConfirm,
  onClose,
}: Readonly<{
  team: TeamRef | null;
  members: number;
  pending: boolean;
  error: unknown;
  onConfirm: (team: TeamRef) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <ConfirmModal
      open={team !== null}
      onClose={onClose}
      title={t("users.archiveTeamTitle", { name: team?.name ?? "" })}
      confirmLabel={t("users.teamArchive")}
      confirmVariant="danger"
      onConfirm={() => team && onConfirm(team)}
      pending={pending}
      error={error ? problemMessageOf(error, t) : null}
    >
      <p>
        {plural("users.archiveTeamBody", members, {
          count: formatNumber(members, locale),
        })}
      </p>
    </ConfirmModal>
  );
}
