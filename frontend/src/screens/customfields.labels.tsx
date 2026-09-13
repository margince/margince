// The words the custom-field admin puts on a type and on an object.
//
// Their own file because customfields.tsx sits at a frozen line ceiling and
// these two are the part of it with no behaviour at all: a message key per
// member, and the Record type is what makes a new field type or a new target
// fail to compile until somebody names it.

import type { useT } from "../i18n";
import type { CfObject, CfType } from "./customfields.logic";

export function typeLabels(t: ReturnType<typeof useT>): Record<CfType, string> {
  return {
    text: t("cf.type.text"),
    number: t("cf.type.number"),
    date: t("cf.type.date"),
    currency: t("cf.type.currency"),
    picklist: t("cf.type.picklist"),
    boolean: t("cf.type.boolean"),
  };
}

export function objectLabels(
  t: ReturnType<typeof useT>,
): Record<CfObject, string> {
  return {
    deal: t("cf.obj.deal"),
    company: t("cf.obj.company"),
    contact: t("cf.obj.contact"),
    lead: t("cf.obj.lead"),
    project: t("cf.obj.project"),
    contract: t("cf.obj.contract"),
  };
}
