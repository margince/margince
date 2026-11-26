// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { faceClass } from "./button";
import { liveDialogs } from "./dialogfocus";

type Submitter = HTMLButtonElement | HTMLInputElement;

function isSubmitter(element: Element): element is Submitter {
  return (
    (element instanceof HTMLButtonElement ||
      element instanceof HTMLInputElement) &&
    element.type === "submit"
  );
}

function formOwning(element: Element | null): HTMLFormElement | null {
  if (
    element instanceof HTMLButtonElement ||
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement
  ) {
    return element.form;
  }
  return element?.closest("form") ?? null;
}

function dialogForm(dialog: HTMLElement): HTMLFormElement | null {
  const focused = formOwning(document.activeElement);
  if (focused !== null && dialog.contains(focused)) {
    return focused;
  }
  const forms = dialog.querySelectorAll("form");
  return forms.length === 1 ? (forms[0] ?? null) : null;
}

function primarySubmit(form: HTMLFormElement): Submitter | null {
  const submits = [...form.elements].filter(isSubmitter);
  const candidates =
    submits.length === 1
      ? submits
      : submits.filter((submit) =>
          submit.classList.contains(faceClass("primary")),
        );
  return candidates.length === 1 ? (candidates[0] ?? null) : null;
}

// The dialog whose handler mounted last need not be the one on top.
export function submitDialogForm(dialog: HTMLElement): void {
  if (liveDialogs().at(-1) !== dialog) {
    return;
  }
  const form = dialogForm(dialog);
  const submit = form === null ? null : primarySubmit(form);
  if (
    form === null ||
    submit === null ||
    submit.disabled ||
    submit.getAttribute("aria-disabled") === "true" ||
    submit.classList.contains(faceClass("danger"))
  ) {
    return;
  }
  form.requestSubmit(submit);
}
