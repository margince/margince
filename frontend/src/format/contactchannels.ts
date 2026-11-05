import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Contact = components["schemas"]["Contact"];
type EmailType = NonNullable<
  NonNullable<Contact["emails"]>[number]["email_type"]
>;
type PhoneType = NonNullable<
  NonNullable<Contact["phones"]>[number]["phone_type"]
>;

// Forms and decision cards use the same names for contact channels.
export const EMAIL_TYPE_LABEL: Readonly<Record<EmailType, MessageKey>> = {
  work: "field.emailWork",
  personal: "field.emailPersonal",
  other: "field.emailOther",
};
export const PHONE_TYPE_LABEL: Readonly<Record<PhoneType, MessageKey>> = {
  work: "field.phoneWork",
  mobile: "field.phoneMobile",
  home: "field.phoneHome",
  other: "field.phoneOther",
};
