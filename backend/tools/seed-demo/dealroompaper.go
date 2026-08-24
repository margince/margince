// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The paper a deal room puts in front of its buyer.
//
// Split from dealrooms.go, which holds the room itself and the conversation
// in it. This half is only about documents: finding the contract the dataset
// names, rendering its page a second time against the DEAL, and telling the
// room to show it.
//
// The second copy is the point of the file. A room document must point at an
// attachment filed on the room's deal (`entity_type: deal`); the contract PDF
// every other phase writes is filed on the ORGANIZATION, because that is
// where a contract's paper belongs and where the account page reads it from.
// The two cannot be one row, and collapsing them would mean removing a
// document from an account silently emptied a room.

import (
	"fmt"
	"net/url"
	"strings"
)

// seedRoomDocuments puts the deal's paper in front of the buyer, uploading a
// deal-scoped copy first because the account's copy is filed elsewhere.
//
// Returns the attachment id per contract ref, so a thread can name the
// document it is about.
func seedRoomDocuments(c *client, room demoDealRoom, roomID, dealID string, refs pipelineRefs) (map[string]string, int, error) {
	byContract := map[string]string{}
	if len(room.Documents) == 0 {
		return byContract, 0, nil
	}

	onFile, err := dealRoomDocuments(c, roomID)
	if err != nil {
		return nil, 0, err
	}

	added := 0
	for i, doc := range room.Documents {
		contract, err := contractByRef(c, refs, doc.Contract)
		if err != nil {
			return nil, added, err
		}
		title := doc.Title
		if title == "" {
			title = contract.Title
		}
		if id, ok := onFile[title]; ok {
			byContract[doc.Contract] = id
			continue
		}

		attachmentID, err := uploadDealCopy(c, contract, dealID, refs)
		if err != nil {
			return nil, added, err
		}
		body := jsonBody{
			"attachment_id": attachmentID,
			"group_key":     doc.Group,
			"title":         title,
			"position":      i,
			"source":        seedSource,
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := c.post("/v1/deal-rooms/"+roomID+"/documents", body, &out); err != nil {
			return nil, added, fmt.Errorf("adding %s: %w", title, err)
		}
		byContract[doc.Contract] = out.ID
		added++
	}
	return byContract, added, nil
}

// dealRoomDocuments is what the room already shows, keyed by the buyer-facing
// title -- which is what the dataset names it by, and what a re-run has to
// recognise so it does not upload a second copy of the same page.
func dealRoomDocuments(c *client, roomID string) (map[string]string, error) {
	var page struct {
		Data []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := c.get("/v1/deal-rooms/"+roomID+"/documents", nil, &page); err != nil {
		return nil, fmt.Errorf("listing the room's documents: %w", err)
	}
	out := make(map[string]string, len(page.Data))
	for _, row := range page.Data {
		out[row.Title] = row.ID
	}
	return out, nil
}

// uploadDealCopy files a contract's page against the DEAL.
//
// The same render the account's copy gets. It is a second attachment row on
// purpose: the account copy is the company's paper and this one is what a
// named buyer was shown in a room, and collapsing them would mean removing a
// document from an account silently emptied a room.
func uploadDealCopy(c *client, contract seededContract, dealID string, refs pipelineRefs) (string, error) {
	body := renderPDF(contractPage(contract, refs))
	attachmentID, err := c.upload("/v1/attachments", documentFilename(contract), body, map[string]string{
		"entity_type": "deal",
		"entity_id":   dealID,
		"contract_id": contract.ID,
	})
	if err != nil {
		return "", fmt.Errorf("uploading the deal copy of %s: %w", contract.Title, err)
	}
	metadata := jsonBody{
		"category":  "contract",
		"title":     contract.Title,
		"doc_state": docStateFor(contract.Status),
	}
	if err := c.patch("/v1/attachments/"+attachmentID+"/metadata", metadata, nil); err != nil {
		return "", fmt.Errorf("setting the deal copy's metadata: %w", err)
	}
	return attachmentID, nil
}

// contractByRef resolves a dataset contract ref to the row on file, by the
// company it names and the title it carries -- the same pair ensureContract
// converges on.
func contractByRef(c *client, refs pipelineRefs, ref string) (seededContract, error) {
	for _, want := range refs.contractsByRef {
		if want.Ref != ref {
			continue
		}
		orgID, ok := refs.orgsByDom[strings.ToLower(want.Company)]
		if !ok {
			return seededContract{}, fmt.Errorf("contract %s names company %q, which is not seeded", ref, want.Company)
		}
		found, err := contractOn(c, orgID, want.Title)
		if err != nil {
			return seededContract{}, err
		}
		if found.ID == "" {
			return seededContract{}, fmt.Errorf("contract %s (%q) is not on file", ref, want.Title)
		}
		return found, nil
	}
	return seededContract{}, fmt.Errorf("no contract has ref %q", ref)
}

func contractOn(c *client, orgID, title string) (seededContract, error) {
	var page struct {
		Data []seededContract `json:"data"`
	}
	// Under the ORGANIZATION, not /v1/contracts: a contract belongs to an
	// account and the workspace-wide path does not accept a GET at all.
	if err := c.get("/v1/organizations/"+orgID+"/contracts", url.Values{"limit": {"50"}}, &page); err != nil {
		return seededContract{}, fmt.Errorf("listing contracts for %s: %w", orgID, err)
	}
	for _, row := range page.Data {
		if strings.EqualFold(row.Title, title) {
			if row.OrganizationID == "" {
				row.OrganizationID = orgID
			}
			return row, nil
		}
	}
	return seededContract{}, nil
}
