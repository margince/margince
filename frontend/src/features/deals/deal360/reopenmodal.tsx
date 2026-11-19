// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { ErrorLine } from "../../../design-system/errorline";
import { useT } from "../../../i18n";
import "./deal360.css";

type Stage = components["schemas"]["Stage"];

/**
 * The question a closed deal is reopened through, from the record page's
 * Reopen and from a drop on an open board column alike. Reopening clears how
 * the deal closed, so both ask the same thing in the same words.
 */
export function ReopenModal({
  open,
  onClose,
  openStages,
  proposedStageId,
  pending,
  error,
  onConfirm,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  openStages: readonly Stage[];
  // The stage chosen when the question opens: the column a card was dropped
  // on. The reader may still pick another.
  proposedStageId?: string;
  pending: boolean;
  error?: unknown;
  onConfirm: (toStage: Stage) => void;
}>) {
  const t = useT();
  const [stageId, setStageId] = useState<string | null>(null);
  // Each opening starts from its own proposal, not from the last answer.
  const [wasOpen, setWasOpen] = useState(false);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setStageId(proposedStageId ?? null);
    }
  }
  const chosen = openStages.find((stage) => stage.id === stageId);
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("deal.reopenPick")}
      confirmLabel={t("deal.reopenConfirm")}
      confirmDisabled={!chosen}
      pending={pending}
      onConfirm={() => {
        if (chosen) {
          onConfirm(chosen);
        }
      }}
    >
      <div className="d360-reopen-stages">
        {openStages.map((s) => (
          <Button
            key={s.id}
            aria-pressed={stageId === s.id}
            data-testid={`reopen-stage-${s.id}`}
            onClick={() => setStageId(s.id)}
          >
            {s.name}
          </Button>
        ))}
      </div>
      <ErrorLine error={error} />
    </ConfirmModal>
  );
}
