// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

const DATE_SCOPE = 'form, [role="dialog"], [data-date-scope]';

// DateInput reports text it cannot read as "", so a save past it erases the day.
// Not checkValidity() on all: native step and required rules belong to the caller and server.
export function refuseUnreadableField(from: Element): boolean {
  const scope = from.closest(DATE_SCOPE);
  if (scope === null) {
    throw new Error(
      `refuseUnreadableField needs a ${DATE_SCOPE} around the save; mark the box that holds its date fields with data-date-scope`,
    );
  }
  const unread = Array.from(scope.querySelectorAll("input")).find(
    (field) => field.validity.customError,
  );
  if (unread === undefined) {
    return false;
  }
  // checkValidity fires the invalid event that draws the field's refusal;
  // reportValidity would add a bubble.
  unread.checkValidity();
  unread.focus();
  return true;
}

// The control a native submit of `form` reports and focuses.
export function firstRefusedControl(
  form: HTMLFormElement,
): Element | undefined {
  return Array.from(form.elements).find(
    (control) =>
      (control instanceof HTMLInputElement ||
        control instanceof HTMLSelectElement ||
        control instanceof HTMLTextAreaElement) &&
      control.willValidate &&
      !control.validity.valid,
  );
}
