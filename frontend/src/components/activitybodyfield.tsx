// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, Textarea } from "../design-system/atoms";
import { RichText } from "../design-system/richtext";
import { useRichTextLabels } from "../lib/richtextlabels";

/**
 * An activity's body as a field: markdown edited as formatted text, as the
 * timeline renders it.
 *
 * A transcript stays a plain textarea and is kept byte for byte. The server's
 * normalizer numbers its lines, and a citation points at those numbers.
 */
export function ActivityBodyField({
  label,
  hint,
  value,
  onChange,
  transcript = false,
  rows,
  grow = false,
  disabled = false,
  className,
}: Readonly<{
  label: string;
  hint?: string;
  value: string;
  onChange: (body: string) => void;
  transcript?: boolean;
  rows: number;
  // Fills the room its host gives it and grows with the text, `rows` the floor.
  grow?: boolean;
  // Held while the text is being saved, so a word typed then is not dropped.
  disabled?: boolean;
  className?: string;
}>) {
  const labels = useRichTextLabels();
  return (
    // A rich surface carries its hint in its own footer, wired to the surface.
    <Field
      label={label}
      hint={transcript ? hint : undefined}
      className={className}
    >
      {(control) =>
        transcript ? (
          <Textarea
            {...control}
            rows={rows}
            disabled={disabled}
            value={value}
            onChange={(event) => onChange(event.target.value)}
          />
        ) : (
          <RichText
            id={control.id}
            format="markdown"
            value={value}
            onChange={(next) => onChange(next.markdown)}
            label={label}
            labels={labels}
            hint={hint}
            rows={rows}
            grow={grow}
            disabled={disabled}
          />
        )
      }
    </Field>
  );
}
