// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { TeamPicker, VisibilityChoice } from "../../components/readerteam";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  blankRequired,
  problemCodeOf,
  problemMessageOf,
  problemStatus,
  unwrap,
} from "../../lib/problem";

type BusinessConnection = components["schemas"]["TelegramBusinessConnection"];
type Team = components["schemas"]["Team"];
type Visibility = "held" | "shared";

const HINTS: Record<Visibility, MessageKey> = {
  held: "connectors.tgbiz.visibilityHeldHint",
  shared: "connectors.tgbiz.visibilitySharedHint",
};

// A 409 is either the owner switching the link off while the dialog was open
// or a team other than the seat's. The server gives no code to tell them
// apart, so one line names both.
function approveError(
  error: unknown,
  t: ReturnType<typeof useT>,
): string | null {
  if (!error) {
    return null;
  }
  if (problemCodeOf(error) === "reader_team_required") {
    return t("connectors.tgbiz.readerTeamRequired");
  }
  if (problemStatus(error) === 409) {
    return t("connectors.tgbiz.approveConflict");
  }
  return problemMessageOf(error, t);
}

/** Approves a link as a company account read by the team the admin picks. */
export function ApproveBusinessDialog({
  connection,
  teams,
  onClose,
  onDone,
  onSwitchedOff,
}: Readonly<{
  connection: BusinessConnection;
  teams: readonly Team[];
  onClose: () => void;
  onDone: () => void;
  onSwitchedOff: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [team, setTeam] = useState(connection.readerTeamId ?? "");
  const approve = useMutation({
    mutationFn: async (readerTeamId: string) =>
      unwrap(
        await api.POST("/channel-connections/telegram-business/{id}/approve", {
          params: { path: { id: connection.id } },
          body: { readerTeamId },
        }),
      ),
    onSuccess: onDone,
    onError: (error) => {
      if (problemStatus(error) === 409) {
        onSwitchedOff();
      }
    },
  });
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("connectors.tgbiz.approveTitle", {
        owner: connection.ownerLabel,
      })}
      confirmLabel={t("connectors.tgbiz.approve")}
      bodyRef={faultsRoot}
      pending={approve.isPending}
      error={approveError(approve.error, t)}
      onConfirm={() => {
        const blank = blankRequired(locale, {
          readerTeamId: [t("sharedMailboxes.readerTeam"), team],
        });
        if (!faults.show(blank)) {
          approve.mutate(team);
        }
      }}
    >
      <div className="form-stack">
        <p>{t("connectors.tgbiz.approveBody")}</p>
        <TeamPicker
          teams={teams}
          value={team}
          onChange={(id) => {
            setTeam(id);
            faults.edited("readerTeamId");
          }}
          error={faults.of("readerTeamId")}
        />
      </div>
    </ConfirmModal>
  );
}

/** Changes who reads an approved company account's messages. */
export function BusinessVisibilityDialog({
  connection,
  onClose,
  onDone,
}: Readonly<{
  connection: BusinessConnection;
  onClose: () => void;
  onDone: () => void;
}>) {
  const t = useT();
  const current: Visibility =
    connection.mailVisibility === "shared" ? "shared" : "held";
  const [visibility, setVisibility] = useState<Visibility>(current);
  const change = useMutation({
    mutationFn: async (next: Visibility) =>
      unwrap(
        await api.PATCH(
          "/channel-connections/telegram-business/{id}/visibility",
          {
            params: { path: { id: connection.id } },
            body: { mail_visibility: next, confirm: true },
          },
        ),
      ),
    onSuccess: onDone,
  });
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("connectors.tgbiz.visibilityTitle", {
        owner: connection.ownerLabel,
      })}
      confirmLabel={t("connectors.tgbiz.visibilityConfirm")}
      confirmDisabled={visibility === current}
      pending={change.isPending}
      error={change.isError ? problemMessageOf(change.error, t) : null}
      onConfirm={() => change.mutate(visibility)}
    >
      <div className="form-stack">
        <VisibilityChoice
          value={visibility}
          onChange={setVisibility}
          hints={HINTS}
        />
        <Callout
          tone="warning"
          title={t("connectors.tgbiz.visibilityWarning")}
        />
      </div>
    </ConfirmModal>
  );
}
