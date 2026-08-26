// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Where each account stands, what we sell, and what we quoted.
//
// Split from records.go, which files what HAPPENED on an account. These three
// phases answer a different question -- not "what was said" but "where does
// this account stand and what is on the table" -- and they run after the
// activities because a lifecycle reads the deals and an offer names one.

import (
	"fmt"
	"net/url"
	"strings"
)

// seedLifecycle says where each account stands with us.
//
// It runs after the deals exist, because the two have to agree: an account
// with a won deal is a customer and one with an open deal is at least an
// opportunity. A demo where every company sits at the default teaches the
// lifecycle filter to return everything.
//
// A company the dataset does not place takes the lifecycle its PROFILE was
// planned with, which is what spreads the accounts across customer, former
// customer, opportunity, prospect and target instead of leaving everything at
// two values. demo.json still overrides whenever the story needs something
// specific, and a company with no profile at all falls back to what its
// records show — an open deal makes it an opportunity, otherwise a target.
func seedLifecycle(c *client, cfg demoConfig, refs pipelineRefs, plan map[string]profile, mode runMode) (int, error) {
	changed := 0
	placed := map[string]bool{}
	for _, domains := range cfg.Lifecycle {
		for _, domain := range domains {
			placed[strings.ToLower(domain)] = true
		}
	}
	for domain := range refs.orgsByDom {
		if placed[domain] {
			continue
		}
		stage := plan[domain].Lifecycle
		if stage == "" {
			stage = "target"
			if len(refs.dealsByCompany[domain]) > 0 {
				stage = "opportunity"
			}
		}
		cfg.Lifecycle[stage] = append(cfg.Lifecycle[stage], domain)
	}
	for stage, domains := range cfg.Lifecycle {
		for _, domain := range domains {
			orgID, ok := refs.orgsByDom[strings.ToLower(domain)]
			if !ok {
				return changed, fmt.Errorf("lifecycle names company %q, which is not seeded", domain)
			}
			if mode == modeDryRun {
				changed++
				continue
			}
			current, version, source, err := organizationLifecycle(c, orgID)
			if err != nil {
				return changed, err
			}
			// A company whose record somebody else owns keeps the lifecycle
			// stage it carries. Moving an account along the pipeline by hand is
			// the demo's whole point; a re-seed must not walk it back.
			if !seederOwns(source) {
				continue
			}
			if current == stage {
				continue
			}
			// The write is version-checked, so a concurrent edit loses rather
			// than being silently overwritten.
			body := jsonBody{"lifecycle": stage, "if_version": version}
			if err := c.patch("/v1/organizations/"+orgID, body, nil); err != nil {
				return changed, fmt.Errorf("setting %s to %s: %w", domain, stage, err)
			}
			changed++
		}
	}
	return changed, nil
}

func organizationLifecycle(c *client, orgID string) (stage string, version int, source string, err error) {
	var out struct {
		Lifecycle string `json:"lifecycle"`
		Source    string `json:"source"`
		Version   int    `json:"version"`
	}
	if err := c.get("/v1/organizations/"+orgID, nil, &out); err != nil {
		return "", 0, "", fmt.Errorf("reading organization %s: %w", orgID, err)
	}
	return out.Lifecycle, out.Version, out.Source, nil
}

// seedProducts fills the rate card the offers draw their line items from.
func seedProducts(c *client, cfg demoConfig, mode runMode) (map[string]string, int, error) {
	ids := map[string]string{}
	created := 0

	var page struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if mode != modeDryRun {
		if err := c.get("/v1/products", url.Values{"limit": {"100"}}, &page); err != nil {
			return nil, 0, fmt.Errorf("listing products: %w", err)
		}
	}
	byName := map[string]string{}
	for _, row := range page.Data {
		byName[strings.ToLower(row.Name)] = row.ID
	}

	for _, product := range cfg.Products {
		if id, ok := byName[strings.ToLower(product.Name)]; ok {
			ids[product.Ref] = id
			continue
		}
		if mode == modeDryRun {
			created++
			continue
		}
		body := jsonBody{
			"name":             product.Name,
			"unit_price_minor": product.UnitPriceMinor,
			"currency":         product.Currency,
			"source":           seedSource,
		}
		addIfSet(body, "sku", product.SKU)
		addIfSet(body, "unit", product.Unit)
		addIfSet(body, "description", product.Description)

		var out struct {
			ID string `json:"id"`
		}
		if err := c.post("/v1/products", body, &out); err != nil {
			return nil, created, fmt.Errorf("product %s: %w", product.Ref, err)
		}
		ids[product.Ref] = out.ID
		created++
	}
	return ids, created, nil
}

// seedOffers quotes the open deals, and drives each offer to its dataset
// state through the real send/accept/reject transitions rather than writing a
// state column: an accepted offer that was never sent is not a state the
// product can reach.
func seedOffers(c *client, cfg demoConfig, refs pipelineRefs, products map[string]string, mode runMode) (int, error) {
	created := 0
	for _, offer := range cfg.Offers {
		dealID, err := dealIDFor(c, cfg, refs, offer.Deal)
		if err != nil {
			return created, err
		}
		if dealID == "" {
			continue // the deal is not seeded (dry run, or a -limit subset)
		}
		if mode == modeDryRun {
			created++
			continue
		}
		if has, err := dealHasOffer(c, dealID); err != nil {
			return created, err
		} else if has {
			continue
		}

		lines := make([]jsonBody, 0, len(offer.Lines))
		for _, line := range offer.Lines {
			productID, ok := products[line.Product]
			if !ok {
				return created, fmt.Errorf("offer %s names product %q, which is not seeded", offer.Ref, line.Product)
			}
			item := jsonBody{"product_id": productID, "quantity": line.Quantity}
			// Only sent when the dataset states one. Otherwise the line takes
			// the product's own price, which is what every same-currency
			// offer wants.
			if line.UnitPriceMinor > 0 {
				item["unit_price_minor"] = line.UnitPriceMinor
			}
			lines = append(lines, item)
		}
		body := jsonBody{"currency": offer.Currency, "source": seedSource, "line_items": lines}
		addIfSet(body, "intro_text", offer.IntroText)
		if offer.ValidInDays != 0 {
			body["valid_until"] = refs.date(offer.ValidInDays)
		}

		var out struct {
			ID string `json:"id"`
		}
		if err := c.post("/v1/deals/"+dealID+"/offers", body, &out); err != nil {
			return created, fmt.Errorf("offer %s: %w", offer.Ref, err)
		}
		created++

		if err := driveOfferTo(c, out.ID, offer.State); err != nil {
			return created, fmt.Errorf("offer %s: %w", offer.Ref, err)
		}
	}
	return created, nil
}

// driveOfferTo walks an offer to its target state. Accept and reject both
// require a sent offer, so the send is not optional scaffolding.
func driveOfferTo(c *client, offerID, state string) error {
	if state == "" || state == "draft" {
		return nil
	}
	if err := c.post("/v1/offers/"+offerID+"/send", jsonBody{}, nil); err != nil {
		return fmt.Errorf("sending: %w", err)
	}
	switch state {
	case "sent":
		return nil
	case "accepted":
		if err := c.post("/v1/offers/"+offerID+"/accept", jsonBody{}, nil); err != nil {
			return fmt.Errorf("accepting: %w", err)
		}
	case "rejected":
		if err := c.post("/v1/offers/"+offerID+"/reject", jsonBody{}, nil); err != nil {
			return fmt.Errorf("rejecting: %w", err)
		}
	default:
		return fmt.Errorf("unknown offer state %q", state)
	}
	return nil
}

func dealIDFor(c *client, cfg demoConfig, refs pipelineRefs, dealRef string) (string, error) {
	for _, deal := range cfg.Deals {
		if deal.Ref != dealRef {
			continue
		}
		orgID, ok := refs.orgsByDom[strings.ToLower(deal.Company)]
		if !ok {
			return "", nil
		}
		return findDeal(c, deal.Name, orgID)
	}
	return "", fmt.Errorf("no deal has ref %q", dealRef)
}

func dealHasOffer(c *client, dealID string) (bool, error) {
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.get("/v1/deals/"+dealID+"/offers", url.Values{"limit": {"5"}}, &page); err != nil {
		return false, fmt.Errorf("listing offers for deal %s: %w", dealID, err)
	}
	return len(page.Data) > 0, nil
}
