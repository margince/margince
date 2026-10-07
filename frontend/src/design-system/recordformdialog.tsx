// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { Modal } from "./atoms";
import { DrawerBody, DrawerFoot, DrawerHead } from "./drawerbands";
import { Heading } from "./heading";
import { intentForFieldCount } from "./modal";

// The create/edit dialog around a record form. Its shape follows the fields it
// counts, taken once per opening: a box changing shape remounts the form.
export function RecordFormDialog({
  open,
  title,
  onClose,
  fieldCount,
  form,
  actions,
}: Readonly<{
  open: boolean;
  title: string;
  onClose: () => void;
  fieldCount: number;
  form: ReactNode;
  actions: ReactNode;
}>) {
  const headingId = useId();
  const live = intentForFieldCount(fieldCount);
  const [opening, setOpening] = useState({ open, shape: live });
  if (opening.open !== open) {
    setOpening({ open, shape: open ? live : opening.shape });
  }
  const shape = open && !opening.open ? live : opening.shape;
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent={shape}>
      {shape === "form" ? (
        <>
          <Heading size="large" id={headingId} className="t-h2 modal-title">
            {title}
          </Heading>
          {form}
          <div className="actions">{actions}</div>
        </>
      ) : (
        <>
          <DrawerHead>
            <Heading size="large" id={headingId} className="t-h2 modal-title">
              {title}
            </Heading>
          </DrawerHead>
          <DrawerBody>{form}</DrawerBody>
          <DrawerFoot className="actions">{actions}</DrawerFoot>
        </>
      )}
    </Modal>
  );
}
