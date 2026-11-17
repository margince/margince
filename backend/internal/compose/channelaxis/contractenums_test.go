// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package channelaxis_test

import (
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type contractEnum struct {
	Path    string
	Members []string
}

func contractEnums(t *testing.T, raw []byte) []contractEnum {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	var found []contractEnum
	var walk func(*yaml.Node, string)
	walk = func(node *yaml.Node, path string) {
		switch node.Kind {
		case yaml.DocumentNode:
			for _, child := range node.Content {
				walk(child, path)
			}
		case yaml.MappingNode:
			for i := 0; i < len(node.Content); i += 2 {
				key, value := node.Content[i].Value, node.Content[i+1]
				next := strings.TrimPrefix(path+"/"+key, "/")
				if key == "enum" && value.Kind == yaml.SequenceNode {
					item := contractEnum{Path: path}
					for _, member := range value.Content {
						item.Members = append(item.Members, member.Value)
					}
					if len(item.Members) == 0 {
						t.Fatalf("%s is empty", next)
					}
					found = append(found, item)
				} else {
					walk(value, next)
				}
			}
		case yaml.SequenceNode:
			for i, child := range node.Content {
				walk(child, path+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(&root, "")
	if len(found) == 0 {
		t.Fatal("contract has no enums to check")
	}
	return found
}

// independentPermissionMeaning admits the agent scope that is named for the
// transport it opens: a permission a passport holds, not a provider vocabulary.
func independentPermissionMeaning(path, provider string) bool {
	if provider != "personal_channels" {
		return false
	}
	switch path {
	case "components/schemas/AgentAdmissionPolicy/properties/scope",
		"components/schemas/IssuePassportRequest/properties/scopes/items",
		"components/schemas/ConsentRequest/properties/scopes/items":
		return true
	}
	return false
}

func independentLinkedInMeaning(path, provider string) bool {
	if provider != "linkedin" {
		return false
	}
	// These fields name profile attributes or writing evidence, never conversation transports.
	switch path {
	case "components/parameters/ContactProfileFieldKey/schema",
		"components/schemas/VCardImportField/properties/field",
		"components/schemas/ContactProfileField/properties/field",
		"components/schemas/SaveContactResearchClaim/properties/field",
		"components/schemas/AttentionPairEvidence/properties/field",
		"components/schemas/VoiceCorpusSource/properties/kind",
		"components/schemas/IngestVoiceCorpusSourceRequest/properties/kind":
		return true
	}
	return false
}

func TestContractEnumCensusKeepsBothStylesAndTheirMeaning(t *testing.T) {
	raw := []byte("components:\n  schemas:\n    First:\n      enum: [message, linkedin]\n    Second:\n      enum:\n        - message\n        - linkedin\n    Wrapper:\n      properties:\n        enum:\n          type: array\n          items: {type: string}\n")
	enums := contractEnums(t, raw)
	if len(enums) != 2 {
		t.Fatalf("found %d enums", len(enums))
	}
	for _, item := range enums {
		if len(item.Members) != 2 || item.Members[1] != "linkedin" {
			t.Fatalf("lost enum: %+v", item)
		}
		if independentLinkedInMeaning(item.Path, "linkedin") {
			t.Fatalf("unknown enum %s escaped", item.Path)
		}
	}
	if !independentLinkedInMeaning("components/schemas/ContactProfileField/properties/field", "linkedin") {
		t.Fatal("profile field lost its independent meaning")
	}
	if independentLinkedInMeaning("components/schemas/ContactProfileField/properties/field", "whatsapp") {
		t.Fatal("profile exception admits another transport")
	}
}
