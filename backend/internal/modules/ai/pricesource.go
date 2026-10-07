// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// priceSourceKind is where the price sync reads one provider's prices. The zero
// value is unset, which TestEveryProviderDeclaresWhereItsPricesComeFrom refuses.
type priceSourceKind uint8

const (
	priceSourceUnset priceSourceKind = iota
	priceFromModelsDev
	priceFromBroker
	priceNotPublished
)

type priceSource struct {
	kind priceSourceKind
	// catalogueKey is the provider's key in models.dev's api.json.
	catalogueKey string
}

func modelsDevSource(key string) priceSource {
	return priceSource{kind: priceFromModelsDev, catalogueKey: key}
}

func brokerPriced(provider string) bool {
	d, _ := providerByName(provider)
	return d.priceSource.kind == priceFromBroker
}

func modelsDevKeyFor(provider string) (string, bool) {
	d, _ := providerByName(provider)
	return d.priceSource.catalogueKey, d.priceSource.kind == priceFromModelsDev
}

func modelsDevProviders() []string {
	return providerNamesWhere(func(d providerDescriptor) bool { return d.priceSource.kind == priceFromModelsDev })
}
