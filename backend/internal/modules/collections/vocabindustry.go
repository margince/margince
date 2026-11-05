// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// industryValues mirrors the industry codes of the contract, since collections
// cannot import the contacts module that owns them. Held against crm.yaml by
// TestEveryOfferedPicklistMatchesTheContractsValues (backend/gates/corepicklistcontract_test.go).
var industryValues = []string{
	"agriculture",
	"automotive",
	"aerospace_defense",
	"chemicals_materials",
	"construction",
	"consumer_goods_retail",
	"education",
	"energy_utilities",
	"entertainment_media",
	"financial_services",
	"food_beverage",
	"government_public_sector",
	"healthcare",
	"hospitality_tourism",
	"insurance",
	"it_services",
	"software",
	"legal_professional_services",
	"logistics_transport",
	"manufacturing",
	"marketing_advertising",
	"mining_metals",
	"nonprofit",
	"pharma_biotech",
	"real_estate",
	"telecommunications",
}
