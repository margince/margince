// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { Button } from "../design-system/atoms";
import { focusableWithin } from "../design-system/dialogfocus";
import { useFocusHandoff } from "../design-system/focushandoff";
import { Heading } from "../design-system/heading";
import { DiscardDraftQuestion } from "../design-system/modal";
import { useT } from "../i18n";
import {
  type CallRecord,
  type CallStart,
  CallStartProvider,
  useCallStart,
} from "../lib/callstart";
import { useCanWrite } from "../lib/capability";
import { UnsavedContext, useUnsavedRegistry } from "../lib/unsavedclaims";
import { EntityRef } from "./entityref";
import { LogActivityForm } from "./logactivity";
import { LogActivityAction } from "./logactivity.action";

type StartedCall = CallRecord & Readonly<{ seq: number }>;

/** The call started last, counted so each press mounts a fresh form. */
function useStartedCall(enabled: boolean): Readonly<{
  call: StartedCall | null;
  start: CallStart | null;
  close: () => void;
}> {
  const [call, setCall] = useState<StartedCall | null>(null);
  const start = useCallback((record: CallRecord) => {
    setCall((prior) => ({ ...record, seq: (prior?.seq ?? 0) + 1 }));
  }, []);
  const close = useCallback(() => setCall(null), []);
  return { call, start: enabled ? start : null, close };
}

/**
 * A surface that logs calls to its own contact in place, and hands any other
 * contact's call on. No start above it means none here either.
 */
export function useCallLogFor(contactId: string | undefined) {
  const outer = useCallStart();
  const { call, start, close } = useStartedCall(outer !== null);
  const routed = useCallback(
    (record: CallRecord) =>
      record.entityId === contactId ? start?.(record) : outer?.(record),
    [contactId, start, outer],
  );
  return { call, start: outer && start ? routed : null, close };
}

// Once, in the shell, so a surface showing a number names its contact and knows
// nothing about log forms. Above the routes, so typed words survive a page move.
export function CallLogHost({ children }: Readonly<{ children: ReactNode }>) {
  // useCanWrite, not useCan: a read seat is refused the POST before RBAC.
  const { call, start, close } = useStartedCall(
    useCanWrite("activity", "create"),
  );
  return (
    <CallStartProvider start={start}>
      {children}
      {call !== null && (
        <LogActivityAction
          key={`${call.entityId}:${call.seq}`}
          entityType={call.entityType}
          entityId={call.entityId}
          askedKind="call"
          openOnMount
          onClose={close}
          about={<EntityRef kind="contact" id={call.entityId} asText />}
        />
      )}
    </CallStartProvider>
  );
}

/** The call log inside a dialog, which must not open a second dialog over itself. */
export function CallLogInline({
  call,
  onClose,
}: Readonly<{ call: StartedCall; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const section = useRef<HTMLElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  // The dialog's draft question would only ask on the dialog's own close.
  const drafts = useUnsavedRegistry(useContext(UnsavedContext));
  const [asking, setAsking] = useState(false);
  const requestClose = () => (drafts.dirty ? setAsking(true) : onClose());
  // biome-ignore lint/correctness/useExhaustiveDependencies: the press count is the trigger, so a second Call brings focus back to the open form.
  useEffect(() => {
    const active = document.activeElement;
    opener.current =
      active instanceof HTMLElement && active !== document.body
        ? active
        : (section.current?.closest<HTMLElement>('[role="dialog"]') ?? null);
    if (section.current) {
      focusableWithin(section.current)[0]?.focus();
    }
  }, [call.seq]);
  useFocusHandoff(section, () =>
    opener.current?.isConnected ? opener.current : null,
  );
  return (
    <section ref={section} aria-labelledby={titleId} className="form-stack">
      <Heading size="small" id={titleId}>
        {t("log.title")}
      </Heading>
      <UnsavedContext.Provider value={drafts}>
        <LogActivityForm
          entityType={call.entityType}
          entityId={call.entityId}
          askedKind="call"
          onLogged={onClose}
          frame={(form, submit) => (
            <>
              {form}
              <div className="form-actions">
                {submit}
                <Button variant="ghost" onClick={requestClose}>
                  {t("common.close")}
                </Button>
              </div>
            </>
          )}
        />
      </UnsavedContext.Provider>
      <DiscardDraftQuestion
        open={asking}
        onKeep={() => setAsking(false)}
        onDiscard={() => {
          onClose();
          opener.current?.focus();
        }}
      />
    </section>
  );
}
