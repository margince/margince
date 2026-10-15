// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode, Ref } from "react";
import { Field, TextInput } from "../design-system/atoms";

/** Whitespace only: an authenticator groups its digits, and no code holds a space. */
export function enteredCode(typed: string) {
  return typed.replace(/\s/g, "");
}

/** One field for an authenticator code or a recovery code, which is case-sensitive. */
export function MfaCodeField({
  label,
  hint,
  error,
  icon,
  ref,
  value,
  onChange,
}: Readonly<{
  label: string;
  hint?: string;
  error?: string;
  icon?: ReactNode;
  ref?: Ref<HTMLInputElement>;
  value: string;
  onChange: (value: string) => void;
}>) {
  return (
    <Field label={label} hint={hint} error={error} icon={icon}>
      {(control) => (
        <TextInput
          {...control}
          ref={ref}
          required
          name="one-time-code"
          autoComplete="one-time-code"
          inputMode="text"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}
