// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import type { EntityKind } from "../lib/entity";
import { LogActivityForm, type OpeningKind } from "./logactivity";

// The same composer as a header button, for a screen whose header is a row of
// actions rather than a column of cards.
export function LogActivityAction({
  entityType,
  entityId,
  askedKind,
  openOnMount,
  triggerIcon,
  disabled,
  disabledReasonId,
  onClose,
  onLogged,
  about,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  // A suggestion opens the kind it already asked for.
  askedKind?: OpeningKind;
  // Rendered already open, with no trigger button, for a caller that is itself
  // the trigger (a suggestion's action) rather than a toolbar offering the verb.
  openOnMount?: boolean;
  // The glyph beside the trigger's words, `aria-hidden` at the call site: a
  // verb is found by its shape first. Optional: `openOnMount` draws no trigger.
  triggerIcon?: ReactNode;
  // Blocks the press with no explanation, for a caller whose grant has not
  // resolved yet. A refusal the server has not decided is worse than a quiet
  // control. It is apart from `disabledReasonId` so a caller can tell the two.
  disabled?: boolean;
  // The sentence that refuses this verb, already on the page. A record that
  // takes no new activity still shows the verb it will not accept. A reader
  // who cannot tell "archived" from "no such button" learns nothing.
  disabledReasonId?: string;
  onClose?: () => void;
  // Only a successful write asks the caller to refresh; dismissal does not.
  onLogged?: () => void;
  // The record named under the title, where the page behind does not show it.
  about?: ReactNode;
}>) {
  const t = useT();
  const titleId = useId();
  const aboutId = useId();
  // Asked for a task, this is Add task: trigger, heading and form all say so.
  const addTask = askedKind === "task";
  const verb = addTask ? "log.addTask" : "log.title";
  const [open, setOpen] = useState(Boolean(openOnMount));
  const close = () => {
    setOpen(false);
    onClose?.();
  };
  return (
    <>
      {!openOnMount && (
        <Button
          disabled={disabled}
          reasonId={disabledReasonId}
          onClick={() => setOpen(true)}
        >
          {triggerIcon}
          {t(verb)}
        </Button>
      )}
      <Modal
        open={open}
        onClose={close}
        labelledBy={titleId}
        describedBy={about ? aboutId : undefined}
        intent="drawer"
      >
        <DrawerHead className="logactivity-head">
          <Heading size="large" id={titleId} className="t-h2">
            {/* The heading answers the verb that opened it: a reader who
                pressed "Add task" reads "Log activity" as the wrong dialog. */}
            {t(verb)}
          </Heading>
          {about && (
            <p id={aboutId} className="t-caption">
              {about}
            </p>
          )}
        </DrawerHead>
        <LogActivityForm
          entityType={entityType}
          entityId={entityId}
          askedKind={askedKind}
          frame={(body, submit) => (
            <>
              <DrawerBody className="logactivity-drawer-body">
                {body}
              </DrawerBody>
              <DrawerFoot className="actions">{submit}</DrawerFoot>
            </>
          )}
          onlyTasks={addTask}
          onLogged={() => {
            onLogged?.();
            close();
          }}
        />
      </Modal>
    </>
  );
}
