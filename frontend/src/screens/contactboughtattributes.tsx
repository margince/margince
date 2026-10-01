import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { FieldRow } from "../design-system/fieldgrid";
import { providerBrandName } from "../design-system/provider-mark";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BoughtText } from "./boughtmarks";

type Profile = components["schemas"]["ContactProviderProfile"];
type Attribute = components["schemas"]["ContactProviderAttribute"];

const KIND_LABEL: Readonly<Record<Attribute["kind"], MessageKey>> = {
  location: "provider.profile.location",
  department: "provider.profile.departments",
  seniority: "provider.profile.seniorities",
};

/**
 * The location, departments and seniority a provider sold, one block per
 * provider in the details card. No record field holds them — "Berlin, Germany"
 * cannot be split into a city and a country without guessing — so they stand
 * as bought facts, each value dated by the run that last reported it.
 */
export function BoughtAttributes({
  profiles,
}: Readonly<{ profiles: readonly Profile[] | undefined }>) {
  const t = useT();
  return (
    <>
      {(profiles ?? []).map((profile) => {
        const lines = attributeFacts(profile, t);
        if (lines.length === 0) {
          return null;
        }
        const provider = profile.provider ?? "";
        return (
          <FieldRow
            key={provider}
            label={t("contact.bought.from", {
              provider: providerBrandName(provider) ?? provider,
            })}
          >
            {lines}
          </FieldRow>
        );
      })}
    </>
  );
}

// One line per kind, the values first and the kind as a caption after them —
// the shape the card already gives an address or a number and its kind.
function attributeFacts(
  profile: Profile,
  t: ReturnType<typeof useT>,
): ReactNode[] {
  return (["location", "department", "seniority"] as const).flatMap((kind) => {
    const values = (profile.attributes ?? []).filter((a) => a.kind === kind);
    if (values.length === 0) {
      return [];
    }
    return [
      <span key={kind} className="fieldgrid-handle">
        <span>
          {values.map((a, index) => (
            <span key={a.value}>
              {index > 0 && ", "}
              <BoughtText
                value={a.value}
                bought={{
                  provider: profile.provider ?? "",
                  applied_at: a.retrieved_at,
                }}
              />
            </span>
          ))}
        </span>
        <span className="t-caption">{t(KIND_LABEL[kind])}</span>
      </span>,
    ];
  });
}
