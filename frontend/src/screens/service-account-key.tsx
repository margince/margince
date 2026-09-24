// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field, Textarea } from "../design-system/atoms";
import { FileDropzone } from "../design-system/filedropzone";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// A Google service-account key file, pasted or picked, for the one vendor that
// takes one. Settings and onboarding both write this credential, so the control
// and its check live here once rather than in each screen.

/**
 * Why a pasted key cannot be sent, or undefined when it can be. A courtesy
 * only: the server parses the file and asks Google, and is the authority on
 * whether the key works. This catches the paste that is not a key file at all.
 */
export function serviceAccountProblem(raw: string): MessageKey | undefined {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return "serviceAccountKey.empty";
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return "serviceAccountKey.notJson";
  }
  if (!isServiceAccountShape(parsed)) {
    return "serviceAccountKey.notServiceAccount";
  }
  return undefined;
}

function isServiceAccountShape(parsed: unknown): boolean {
  if (typeof parsed !== "object" || parsed === null) {
    return false;
  }
  const file = new Map(Object.entries(parsed));
  return (
    file.get("type") === "service_account" &&
    typeof file.get("client_email") === "string" &&
    typeof file.get("private_key") === "string"
  );
}

/**
 * The paste box and the file picker, writing one value. The picked file is
 * read here, in the browser, into the same text the box holds, so what is sent
 * is always what the reader can see and correct.
 */
export function ServiceAccountKeyField({
  value,
  onChange,
  disabled,
  hint,
  error,
}: Readonly<{
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  hint?: string;
  error?: string;
}>) {
  const t = useT();
  const [picked, setPicked] = useState<File | undefined>();
  const [unreadable, setUnreadable] = useState(false);
  return (
    <>
      <Field
        label={t("serviceAccountKey.label")}
        hint={hint}
        error={unreadable ? t("serviceAccountKey.unreadable") : error}
      >
        {(control) => (
          <Textarea
            {...control}
            // The key is never rendered back, so this only ever holds what
            // the reader is about to send; spellcheck would ship it to a
            // browser dictionary service.
            spellCheck={false}
            autoComplete="off"
            rows={6}
            value={value}
            disabled={disabled}
            placeholder={t("serviceAccountKey.placeholder")}
            onChange={(e) => {
              setUnreadable(false);
              onChange(e.target.value);
            }}
          />
        )}
      </Field>
      {!disabled && (
        <FileDropzone
          label={t("serviceAccountKey.fileLabel")}
          emptyLabel={t("serviceAccountKey.fileEmpty")}
          accept=".json,application/json"
          file={picked}
          onPick={(file) => {
            setPicked(file);
            file.text().then(
              (text) => {
                setUnreadable(false);
                onChange(text);
              },
              () => setUnreadable(true),
            );
          }}
        />
      )}
    </>
  );
}
