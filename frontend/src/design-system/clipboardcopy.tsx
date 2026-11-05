// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check, Copy } from "lucide-react";
import {
  type ReactElement,
  type Ref,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { Callout } from "./callout";
import { IconAction } from "./iconaction";

type CopySource =
  | string
  | Readonly<{ key: string; read: () => Promise<string> }>;

// The copied claim follows the source identity. A lazy report is read only
// after the click, so viewing a page cannot trigger diagnostic work.
export function useClipboardCopy(
  text: CopySource,
  labels: Readonly<{
    copy: string;
    copied: string;
    remedy: string;
    pending?: string;
    failureTitle?: string;
  }>,
  onCopied?: () => void,
): Readonly<{
  label: string;
  copied: boolean;
  pending: boolean;
  preparedText: string | null;
  copy: () => void;
  notice: ReactElement | null;
}> {
  const t = useT();
  const [written, setWritten] = useState<string | null>(null);
  const [prepared, setPrepared] = useState<Readonly<{
    identity: string;
    text: string;
  }> | null>(null);
  const [failed, setFailed] = useState<Readonly<{
    identity: string;
    lazy: boolean;
  }> | null>(null);
  const [pendingIdentity, setPending] = useState<string | null>(null);
  const attempt = useRef(0);
  const identity = JSON.stringify([
    typeof text,
    typeof text === "string" ? text : text.key,
  ]);
  const pending = pendingIdentity === identity;
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new report identity cancels the previous unfinished read.
  useLayoutEffect(
    () => () => {
      attempt.current++;
      setPending(null);
    },
    [identity],
  );
  const copied = !pending && written === identity;

  function copy() {
    const mine = ++attempt.current;
    setFailed(null);
    setPrepared(null);
    setPending(identity);
    async function write() {
      const value = typeof text === "string" ? text : await text.read();
      if (mine !== attempt.current) return;
      setPrepared({ identity, text: value });
      const writer = navigator.clipboard;
      if (!writer) throw new Error("Clipboard unavailable");
      await writer.writeText(value);
      if (mine !== attempt.current) return;
      setWritten(identity);
      setPending(null);
      return true;
    }
    write().then(
      (landed) => {
        if (landed && mine === attempt.current) onCopied?.();
      },
      () => {
        if (mine !== attempt.current) return;
        setWritten(null);
        setPending(null);
        setFailed({ identity, lazy: typeof text !== "string" });
      },
    );
  }

  return {
    label: pending
      ? (labels.pending ?? labels.copy)
      : copied
        ? labels.copied
        : labels.copy,
    copied,
    pending,
    preparedText: prepared?.identity === identity ? prepared.text : null,
    copy,
    notice:
      failed !== null &&
      (typeof text === "string"
        ? !failed.lazy
        : failed.identity === identity) ? (
        <Callout
          tone="danger"
          kind="outcome"
          title={labels.failureTitle ?? t("clipboard.copyFailedTitle")}
        >
          {labels.remedy}
        </Callout>
      ) : null,
  };
}

/**
 * The text a copy control copies, drawn so a reader can still take it by hand
 * when the clipboard refuses: it wraps in full, takes keyboard focus, and
 * focus selects all of it, so Tab then the platform's copy keys is the way out.
 */
export function CopyableText({
  text,
  label,
  testId,
  ref,
}: Readonly<{
  text: string;
  label: string;
  testId?: string;
  ref?: Ref<HTMLDivElement>;
}>) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: a textarea does not grow to its text, and the value must show whole at any width
    <div
      ref={ref}
      className="code-block"
      role="textbox"
      aria-readonly="true"
      aria-multiline="true"
      aria-label={label}
      tabIndex={0}
      data-testid={testId}
      onFocus={(event) =>
        document.getSelection()?.selectAllChildren(event.currentTarget)
      }
    >
      {text}
    </div>
  );
}

/**
 * The copy verb as a glyph inside a line of text, beside the value it copies
 * (an address in a record's details). Its label says Copied once the write
 * lands, and its failure notice follows it.
 */
export function CopyAction({
  text,
  labels,
}: Readonly<{
  text: string;
  labels: Readonly<{ copy: string; copied: string; remedy: string }>;
}>) {
  const copy = useClipboardCopy(text, labels);
  return (
    <span className="copy-action">
      <IconAction
        inline
        label={copy.label}
        icon={
          copy.copied ? (
            <Check aria-hidden="true" />
          ) : (
            <Copy aria-hidden="true" />
          )
        }
        onClick={copy.copy}
      />
      {copy.notice}
    </span>
  );
}
