// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useRef } from "react";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Stack } from "../../design-system/stack";

/**
 * The frame every evidence drawer on Analytics shares: the figure's name, the
 * lines that say what it was read for, then its body. Focus starts on the
 * name and returns to the control that opened it.
 */
export function EvidenceFrame({
  title,
  lines,
  onClose,
  children,
}: Readonly<{
  title: string;
  lines: readonly string[];
  onClose: () => void;
  children: ReactNode;
}>) {
  const titleId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      initialFocusTo={() => heading.current}
    >
      <DrawerHead>
        <Heading
          ref={heading}
          tabIndex={-1}
          id={titleId}
          as="h2"
          size="medium"
          className="modal-title"
        >
          {title}
        </Heading>
        {lines.map((line) => (
          <p key={line} className="t-caption">
            {line}
          </p>
        ))}
      </DrawerHead>
      <DrawerBody>
        <Stack gap="4">{children}</Stack>
      </DrawerBody>
    </Modal>
  );
}
