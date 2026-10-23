// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";

/**
 * A reading drawer over the project page, for what the page reaches but does
 * not draw. That is the full history, the files, the companies and contacts on it. The
 * body mounts only while open, so a reader who never opens it pays no read.
 */
export function ProjectDrawer({
  open,
  title,
  onClose,
  children,
}: Readonly<{
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
}>) {
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {title}
        </Heading>
      </DrawerHead>
      <DrawerBody>{open && children}</DrawerBody>
    </Modal>
  );
}
