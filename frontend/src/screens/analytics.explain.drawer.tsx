// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Info } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { Modal } from "../design-system/modal";
import { Row } from "../design-system/stack";
import { useT } from "../i18n";

/**
 * The trigger beside one figure and the drawer it opens: an inline glyph named
 * for the figure, and a right-placed dialog titled the same way. `body` is
 * called with a count of the opens, so each open can mount a fresh read that
 * takes the handle and frame of that moment, even while the last close is
 * still animating. `children` is the cell the trigger sits beside.
 */
export function ExplainDrawer({
  figure,
  body,
  children,
}: Readonly<{
  figure: string;
  body: (opens: number) => ReactNode;
  children?: ReactNode;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [opens, setOpens] = useState(0);
  const titleId = useId();
  const figureId = useId();
  const trigger = (
    <>
      <IconAction
        inline
        label={t("explain.cell", { figure })}
        icon={<Info aria-hidden />}
        onClick={() => {
          setOpens((count) => count + 1);
          setOpen(true);
        }}
      />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={`${titleId} ${figureId}`}
        placement="right"
      >
        <Heading size="large" id={titleId}>
          {t("explain.title")}
        </Heading>
        <p className="t-label" id={figureId}>
          {figure}
        </p>
        {body(opens)}
      </Modal>
    </>
  );
  if (children === undefined) {
    return trigger;
  }
  return (
    <Row gap="1" wrap={false}>
      {children}
      {trigger}
    </Row>
  );
}
