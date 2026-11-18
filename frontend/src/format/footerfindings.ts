import type { components } from "../api/schema";
import type { Locale, Translator } from "../i18n";

import { languageName } from "./languagename";

type Finding = components["schemas"]["FooterFinding"];

export function footerFindingsText(
  findings: readonly Finding[],
  t: Translator,
  locale: Locale,
): string {
  const items = new Set(
    findings.map((finding) => {
      let detail = footerKindText(finding.kind, t);
      if (finding.language)
        detail = t("sendPermission.footer.language", {
          detail,
          language: languageName(finding.language, locale),
        });
      if (finding.missing_means?.length)
        detail = t("sendPermission.footer.means", {
          detail,
          methods: finding.missing_means
            .map((method) => {
              switch (method) {
                case "email":
                  return t("sendPermission.footer.email");
                case "phone":
                  return t("sendPermission.footer.phone");
                case "web":
                  return t("sendPermission.footer.web");
                default:
                  return t("sendPermission.footer.unknown");
              }
            })
            .join(", "),
        });
      return detail;
    }),
  );
  return t("sendPermission.footer.omits", { items: [...items].join(", ") });
}

function footerKindText(kind: string, t: Translator): string {
  switch (kind) {
    case "controller_identity":
      return t("sendPermission.footer.identity");
    case "privacy_contact":
      return t("sendPermission.footer.privacy");
    case "advertiser_contact":
      return t("sendPermission.footer.advertiser");
    case "objection_route":
      return t("sendPermission.footer.objection");
    case "recipient_private_link":
      return t("sendPermission.footer.privateLink");
    case "public_opt_out_link":
      return t("sendPermission.footer.publicLink");
    default:
      return t("sendPermission.footer.unknown");
  }
}
