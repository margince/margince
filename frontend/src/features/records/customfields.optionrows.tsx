// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Button, TextInput } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";

// The choices of a picklist or multiselect, one text row each, as the add-field
// builder and the choices editor both edit them. `aside` draws next to a row,
// such as how many records hold that choice.
export function OptionRows({
  options,
  onChange,
  aside,
}: Readonly<{
  options: string[];
  onChange: (options: string[]) => void;
  aside?: (option: string) => ReactNode;
}>) {
  const t = useT();
  const toast = useToast();
  const remove = (idx: number) => {
    // The last option row is a floor, not a delete target. The refusal shows
    // as a toast instead of being swallowed.
    if (options.length <= 1) {
      // `danger`: this is a refusal, and the completion dot the default tone
      // draws said the opposite of what the sentence says.
      toast.show(t("cf.lastOptionBlocked"), { tone: "danger" });
      return;
    }
    onChange(options.filter((_, i) => i !== idx));
  };
  return (
    <div className="field">
      <span className="t-label">{t("cf.options")}</span>
      <div className="cf-options">
        {options.map((option, idx) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: option rows are positional, not identity-keyed
          <div className="cf-option-row" key={idx}>
            <TextInput
              aria-label={t("cf.optionPlaceholder")}
              placeholder={t("cf.optionPlaceholder")}
              value={option}
              onChange={(event) =>
                onChange(
                  options.map((one, i) =>
                    i === idx ? event.target.value : one,
                  ),
                )
              }
            />
            {aside?.(option)}
            <Button
              iconOnly
              aria-label={t("cf.removeOption")}
              onClick={() => remove(idx)}
            >
              <X aria-hidden="true" />
            </Button>
          </div>
        ))}
      </div>
      <Button onClick={() => onChange([...options, ""])}>
        {t("cf.addOption")}
      </Button>
    </div>
  );
}
