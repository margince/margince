// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { ActionRow } from "../../design-system/actionrow";
import { Button, Modal } from "../../design-system/atoms";
import { ChoiceList } from "../../design-system/choicelist";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import type { Service } from "../../lib/connect-flow-draft";
import { serviceChoices } from "../../lib/connect-services";

// "Change what it brings in" for a Google or Microsoft 365 account: the two
// services ticked as they stand. Ticking one asks the vendor for it; unticking
// one stops it, after the same confirm a disconnect asks.

export type BringsInChange = Readonly<{
  added: readonly Service[];
  removed: readonly Service[];
}>;

export function BringsInDialog({
  open,
  onClose,
  accountName,
  vendorName,
  present,
  pending,
  error,
  onSave,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  accountName: string;
  vendorName: string;
  /** The services the account brings in now. */
  present: readonly Service[];
  /** A consent for an added service is on its way to the vendor. */
  pending: boolean;
  error: unknown;
  onSave: (change: BringsInChange) => void;
}>) {
  const t = useT();
  const headingId = useId();
  const [picked, setPicked] = useState<readonly Service[]>(present);
  // Every opening starts from what the account brings in now.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setPicked(present);
    }
  }
  const change: BringsInChange = {
    added: picked.filter((service) => !present.includes(service)),
    removed: present.filter((service) => !picked.includes(service)),
  };
  const unchanged = change.added.length === 0 && change.removed.length === 0;
  // An addition alone goes straight to the vendor, so the button says where.
  const leaves = change.added.length > 0 && change.removed.length === 0;
  // No roster passed: a service it brings in now is ticked, never refused.
  const choices = serviceChoices(t);
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("connectors.bringsIn.title", { account: accountName })}
      </Heading>
      <div className="form-stack">
        <ChoiceList
          multiple
          legend={t("connectFlow.stepBringIn")}
          hideLegend
          layout="cards"
          choices={choices}
          value={picked}
          onChange={setPicked}
        />
        {change.added.length > 0 && (
          <p className="t-caption">
            {t("connectors.bringsIn.addNote", { vendor: vendorName })}
          </p>
        )}
        <ErrorLine error={error} />
        <ActionRow
          primary={
            <Button
              variant="primary"
              pending={pending}
              reason={
                unchanged ? t("connectors.bringsIn.unchanged") : undefined
              }
              onClick={() => onSave(change)}
            >
              {leaves
                ? t("connectFlow.continueTo", { vendor: vendorName })
                : t("connectors.bringsIn.save")}
            </Button>
          }
        >
          <Button onClick={onClose} disabled={pending}>
            {t("create.cancel")}
          </Button>
        </ActionRow>
      </div>
    </Modal>
  );
}
