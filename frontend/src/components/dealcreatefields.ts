import type { MessageKey } from "../i18n/en";
import {
  acquisitionOptions,
  useAcquisitionSources,
} from "../lib/acquisitionsources.queries";
import type { CreateField } from "./create";
import {
  partnerFields,
  useNoPartnersYet,
  usePartnerOptions,
} from "./dealpartnerfields";
import {
  type ProjectOwnerPicker,
  projectOwnerField,
} from "./projectownerfield";

export function useDealCreateFields(
  t: (key: MessageKey) => string,
  owner: ProjectOwnerPicker,
  partners?: { value: string; label: string }[],
): CreateField[] {
  const fetchedPartners = usePartnerOptions([]);
  const noPartnersYet = useNoPartnersYet();
  const sources = useAcquisitionSources().data;
  return [
    ...projectOwnerField(t, owner, {
      key: "owner_id",
      label: "list.owner",
      mode: "create",
    }),
    {
      key: "acquisition_source",
      label: "deal.acquisitionSource",
      type: "select",
      options: acquisitionOptions(t, sources ?? [], null),
    },
    ...partnerFields(t, partners ?? fetchedPartners, {
      noneYet: noPartnersYet,
    }),
  ];
}
