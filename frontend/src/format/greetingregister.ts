import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Contact = components["schemas"]["Contact"];

export const GREETING_REGISTER_LABEL: Readonly<
  Record<NonNullable<Contact["greeting_register"]>, MessageKey>
> = {
  formal: "create.greetingRegister.formal",
  familiar: "create.greetingRegister.familiar",
};
