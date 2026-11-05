// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import type { Ref } from "react";
import { useT } from "../i18n";
import { Field, TextInput } from "./atoms";
import { Callout } from "./callout";
import "./confirmphrase.css";

/**
 * Whether the typed text is the phrase. The typed side is trimmed and both
 * sides are NFC-normalised, so a composed and a decomposed letter agree.
 * The compare is case-sensitive, like the server's check on the reset phrase.
 * An empty phrase matches nothing, so a missing name can never confirm itself.
 */
export function phraseMatches(value: string, phrase: string): boolean {
  const wanted = phrase.normalize("NFC");
  return wanted !== "" && value.normalize("NFC").trim() === wanted;
}

/**
 * A typed confirmation for a change that cannot be undone or that alters work
 * for everyone. It says what will happen, then asks for the exact phrase.
 *
 * A box, not a dialog. The caller seats it in a ConfirmModal and wires
 * `confirmDisabled={!phraseMatches(value, phrase)}` and `initialFocusTo` to
 * `inputRef`.
 *
 * Without a `title` the danger plate is left out, since the dialog around the
 * box already says what happens.
 */
export function ConfirmPhraseBox({
  title,
  impact,
  prompt,
  phrase,
  value,
  onChange,
  inputRef,
  disabled,
}: Readonly<{
  /** What the change does, in one sentence, as the plate's heading. */
  title?: string;
  /** The consequences, one per bullet, under the title. */
  impact?: readonly string[];
  /** The words before the phrase, ending in a colon: "Type this company name:". */
  prompt: string;
  phrase: string;
  value: string;
  onChange: (next: string) => void;
  inputRef?: Ref<HTMLInputElement>;
  disabled?: boolean;
}>) {
  const t = useT();
  const matched = phraseMatches(value, phrase);
  return (
    <div className="confirm-phrase">
      {title !== undefined && (
        <Callout tone="danger" kind="standing" title={title}>
          {impact !== undefined && impact.length > 0 ? (
            <ul className="confirm-phrase-impact">
              {impact.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : undefined}
        </Callout>
      )}
      <div className="confirm-phrase-entry">
        <Field
          label={
            <>
              {prompt}{" "}
              <strong className="confirm-phrase-phrase">{phrase}</strong>
            </>
          }
        >
          {(control) => (
            <TextInput
              {...control}
              ref={inputRef}
              type="text"
              value={value}
              disabled={disabled}
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              onChange={(event) => onChange(event.target.value)}
            />
          )}
        </Field>
        {/* Mounted while empty: a live region that appears together with its
            words is not announced by every screen reader. */}
        <p className="confirm-phrase-match" role="status">
          {matched && (
            <>
              <Check size={14} aria-hidden="true" />
              {t("confirmPhrase.matches")}
            </>
          )}
        </p>
      </div>
    </div>
  );
}
