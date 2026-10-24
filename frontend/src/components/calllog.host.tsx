// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ReactNode,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { Button } from "../design-system/atoms";
import { focusableWithin } from "../design-system/dialogfocus";
import { useFocusHandoff } from "../design-system/focushandoff";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import {
  type CallRecord,
  type CallStart,
  CallStartProvider,
} from "../lib/callstart";
import { useCanWrite } from "../lib/capability";
import { LogActivityAction, LogActivityForm } from "./logactivity";

type StartedCall = CallRecord & Readonly<{ seq: number }>;

/** The call started last, counted so each press mounts a fresh form. */
export function useStartedCall(enabled: boolean): Readonly<{
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

// Once, in the shell, so a surface showing a number names its contact and knows
// nothing about log forms.
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
        />
      )}
    </CallStartProvider>
  );
}

/** The call log inside a dialog, which must not open a second dialog over itself. */
export function CallLogInline({
  call,
  onClose,
}: Readonly<{ call: CallRecord; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const section = useRef<HTMLElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    const active = document.activeElement;
    opener.current =
      active instanceof HTMLElement && active !== document.body
        ? active
        : (section.current?.closest<HTMLElement>('[role="dialog"]') ?? null);
    if (section.current) {
      focusableWithin(section.current)[0]?.focus();
    }
  }, []);
  useFocusHandoff(section, () =>
    opener.current?.isConnected ? opener.current : null,
  );
  return (
    <section ref={section} aria-labelledby={titleId} className="form-stack">
      <Heading size="small" id={titleId}>
        {t("log.title")}
      </Heading>
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
              <Button variant="ghost" onClick={onClose}>
                {t("common.close")}
              </Button>
            </div>
          </>
        )}
      />
    </section>
  );
}
