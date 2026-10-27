import { api } from "../api/client";
import type { components } from "../api/schema";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { unwrap } from "../lib/problem";
import { CreateAction, type CreateField } from "./create";
import { searchCompanies } from "./filterreference";
import { leadHeldNoticeFor } from "./leadheldnotice";
import { normalizeProfileUrl } from "./profileurl";

type Contact = components["schemas"]["Contact"];

// Quick capture asks for the six things a reader of a public profile can
// state, and nothing else. It is not contactCreateFields trimmed: those
// repeatable email and phone rows cost two clicks before a value can be typed.
//
// The company box also offers companies whose name is like the one typed.
// Picking one attaches it by id, and an unpicked name creates a company, so a
// reader never has to merge one away later.
//
// On a company's page the company is already chosen, so the box is left out.
function quickCaptureFields(askCompany: boolean): CreateField[] {
  const company: CreateField = {
    key: "company_name",
    label: "create.companyName",
    offers: {
      search: searchCompanies,
      pickedKey: "company_id",
      pickedHint: "create.companyPicked",
      newHint: "create.companyNew",
    },
  };
  return [
    { key: "full_name", label: "create.fullName", required: true },
    { key: "title", label: "create.contactTitle" },
    ...(askCompany ? [company] : []),
    { key: "profile_url", label: "create.linkedin" },
    { key: "email", label: "create.email", type: "email" },
    { key: "phone", label: "create.phone" },
  ];
}

// A blank box is a field the reader left alone, not a value they cleared.
// This path only creates, so an empty string is omitted, never sent as null.
function statedValue(values: Record<string, string>, key: string) {
  const value = values[key]?.trim();
  return value ? value : undefined;
}

async function quickCaptureContact(
  values: Record<string, string>,
  t: (key: MessageKey) => string,
  convertLeadIds?: string[],
): Promise<Contact> {
  const data = unwrap(
    await api.POST("/contacts/quick-capture", {
      body: {
        full_name: values.full_name?.trim() ?? "",
        title: statedValue(values, "title"),
        // The contract lets an id win over a name; sending only one of them
        // keeps the request saying what the reader chose.
        ...companyChoice(values),
        // Normalized here, as the contact rail does on save. A bare
        // `linkedin.com/in/jdoe` stored as typed is unlinkable on every surface.
        profile_url: profileUrlOrUndefined(values.profile_url),
        email: statedValue(values, "email"),
        phone: statedValue(values, "phone"),
        convert_lead_ids: convertLeadIds,
      },
    }),
    t,
  );
  return data.contact;
}

function companyChoice(values: Record<string, string>) {
  const companyId = statedValue(values, "company_id");
  return companyId
    ? { company_id: companyId }
    : { company_name: statedValue(values, "company_name") };
}

function profileUrlOrUndefined(raw: string | undefined) {
  const stated = raw?.trim();
  return stated ? normalizeProfileUrl(stated) : undefined;
}

// The leads a resend names to convert, read back off the notice's extra fields.
function leadIdsOf(extra: Record<string, unknown> | undefined) {
  const ids = extra?.convert_lead_ids;
  return Array.isArray(ids)
    ? ids.filter((id): id is string => typeof id === "string")
    : undefined;
}

export type QuickCaptureCompany = Readonly<{
  id: string;
  // The list on the company page that shows the new contact.
  listKey: string;
}>;

// A company's Contacts tab offers quick capture alone: it is the one create
// that writes the contact and its employment in one transaction.
export function QuickCaptureAction({
  company,
}: Readonly<{ company?: QuickCaptureCompany }>) {
  const t = useT();
  // The form stays open and gives no other feedback, so without this six
  // saved contacts look the same as six that failed silently.
  const toast = useToast();
  return (
    <CreateAction
      label={t("create.quickCapture")}
      invalidate={company?.listKey ?? "contacts"}
      screen="contacts"
      testId="quick-capture"
      keepOpen
      create={(values, _rows, _key, extra) =>
        quickCaptureContact(
          company ? { ...values, company_id: company.id } : values,
          t,
          leadIdsOf(extra),
        )
      }
      refusalNotice={leadHeldNoticeFor}
      onCreated={(contact) =>
        toast.show(t("create.quickCaptureSaved", { name: contact.full_name }))
      }
      resolveExisting={(_code, id) => ({ screen: "contacts", id })}
      fields={quickCaptureFields(company === undefined)}
    />
  );
}
