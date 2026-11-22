// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "strings"

// azureRegionCountries is the ISO 3166 country each Azure region sits in, taken
// from Microsoft's public list of Azure regions and geographies. A region this
// build does not list is refused rather than guessed, so data cannot leave the
// agreed country through a region nobody mapped.
var azureRegionCountries = map[string]string{
	"eastus": "US", "eastus2": "US", "eastus3": "US", "southcentralus": "US", "westus": "US",
	"westus2": "US", "westus3": "US", "centralus": "US", "northcentralus": "US", "westcentralus": "US",
	"canadacentral": "CA", "canadaeast": "CA",
	"brazilsouth": "BR", "brazilsoutheast": "BR",
	"mexicocentral": "MX", "chilecentral": "CL",
	"northeurope": "IE", "westeurope": "NL",
	"francecentral": "FR", "francesouth": "FR",
	"germanywestcentral": "DE", "germanynorth": "DE",
	"swedencentral": "SE", "swedensouth": "SE",
	"norwayeast": "NO", "norwaywest": "NO",
	"switzerlandnorth": "CH", "switzerlandwest": "CH",
	"uksouth": "GB", "ukwest": "GB",
	"italynorth": "IT", "polandcentral": "PL", "spaincentral": "ES", "belgiumcentral": "BE",
	"japaneast": "JP", "japanwest": "JP",
	"koreacentral": "KR", "koreasouth": "KR",
	"southeastasia": "SG", "eastasia": "HK",
	"australiaeast": "AU", "australiasoutheast": "AU", "australiacentral": "AU", "australiacentral2": "AU",
	"newzealandnorth": "NZ",
	"centralindia":    "IN", "southindia": "IN", "westindia": "IN", "jioindiawest": "IN", "jioindiacentral": "IN",
	"indonesiacentral": "ID", "malaysiawest": "MY",
	"uaenorth": "AE", "uaecentral": "AE", "qatarcentral": "QA", "israelcentral": "IL",
	"southafricanorth": "ZA", "southafricawest": "ZA",
}

// euMemberCountries are the EU member states by ISO 3166 code. Norway,
// Switzerland and the United Kingdom are European and outside the EU, matching
// how residency.go treats Vertex's London and Zürich.
var euMemberCountries = map[string]bool{
	"AT": true, "BE": true, "BG": true, "HR": true, "CY": true, "CZ": true, "DK": true, "EE": true,
	"FI": true, "FR": true, "DE": true, "GR": true, "HU": true, "IE": true, "IT": true, "LV": true,
	"LT": true, "LU": true, "MT": true, "NL": true, "PL": true, "PT": true, "RO": true, "SK": true,
	"SI": true, "ES": true, "SE": true,
}

// azureRegionCountry is the country of an Azure region, and false for a region
// this build does not know.
func azureRegionCountry(region string) (string, bool) {
	country, known := azureRegionCountries[canonicalAzureRegion(region)]
	return country, known
}

// canonicalAzureRegion is the spelling a region is looked up and stored under.
// Azure names regions in lower case; a pasted name may differ in case or padding.
func canonicalAzureRegion(region string) string {
	return strings.ToLower(strings.TrimSpace(region))
}

// canonicalLocation is the stored spelling of provider's location. Only Azure
// regions are case-folded; a Vertex location stays as written.
func canonicalLocation(provider, location string) string {
	if provider == providerAzureOpenAI {
		return canonicalAzureRegion(location)
	}
	return location
}

// azureRegionInEU reports whether a known Azure region sits in an EU member
// state; an unknown region is not.
func azureRegionInEU(region string) bool {
	country, known := azureRegionCountry(region)
	return known && euMemberCountries[country]
}
