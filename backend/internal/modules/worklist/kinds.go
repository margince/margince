// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package worklist

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

// Kind is one entry of api/worklist-kinds.yaml. Pointer fields tell a field
// left out from one set to its zero value, so an omission is refused.
type Kind struct {
	Name         string
	Category     string    `yaml:"category"`
	Destination  string    `yaml:"destination"`
	Owner        []string  `yaml:"owner"`
	Personal     *bool     `yaml:"personal"`
	DoneInPlace  *bool     `yaml:"done_in_place"`
	Delivery     string    `yaml:"delivery"`
	Producer     *Producer `yaml:"producer"`
	MaxLevel     string    `yaml:"max_level"`
	ExtensionKey bool      `yaml:"-"`
}

// Producer is what rebuilds a pushed kind and which stored times its read
// judges against the clock.
type Producer struct {
	Events []string `yaml:"events"`
	Sweep  string   `yaml:"sweep"`
	Clock  []string `yaml:"clock"`
}

// Catalog is every declared kind, by name.
type Catalog map[string]Kind

var (
	categories   = []string{"customer_waiting", "leads", "deals_at_risk", "meetings", "tasks", "decisions", "relationships", "system"}
	destinations = []string{"today", "review", "system_health", "none"}
	owners       = []string{"named", "nobody", "the_reader", "from_the_deal", "unreadable"}
	deliveries   = []string{"pull", "push"}
	// An extension row may wait on a decision or be agreed work, never claim
	// that a customer is waiting or that it is pinned.
	extensionLevels = []string{"agreed", "blocking", "routine"}
	coreName        = regexp.MustCompile(`^[a-z][a-z_]*$`)
	extensionName   = regexp.MustCompile(`^ext\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
)

// LoadCatalog parses and checks a kinds document. It refuses unknown fields,
// a missing field, a value outside its closed set and an extension kind that
// claims more than an extension may.
func LoadCatalog(doc []byte) (Catalog, error) {
	var parsed struct {
		Kinds map[string]Kind `yaml:"kinds"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(doc))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("worklist kinds: %w", err)
	}
	if len(parsed.Kinds) == 0 {
		return nil, fmt.Errorf("worklist kinds: the document declares no kinds")
	}
	catalog := Catalog{}
	names := make([]string, 0, len(parsed.Kinds))
	for name := range parsed.Kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		kind := parsed.Kinds[name]
		kind.Name = name
		kind.ExtensionKey = extensionName.MatchString(name)
		if err := kind.check(); err != nil {
			return nil, fmt.Errorf("worklist kind %q: %w", name, err)
		}
		catalog[name] = kind
	}
	return catalog, nil
}

func (k Kind) check() error {
	if !k.ExtensionKey && !coreName.MatchString(k.Name) {
		return fmt.Errorf("a name is a core word or ext.<unit>.<kind>")
	}
	if err := oneOf("category", k.Category, categories); err != nil {
		return err
	}
	if err := oneOf("destination", k.Destination, destinations); err != nil {
		return err
	}
	if len(k.Owner) == 0 {
		return fmt.Errorf("owner names at least one answer")
	}
	for _, answer := range k.Owner {
		if err := oneOf("owner", answer, owners); err != nil {
			return err
		}
	}
	if k.Personal == nil || k.DoneInPlace == nil {
		return fmt.Errorf("personal and done_in_place are both stated, never defaulted")
	}
	if err := oneOf("delivery", k.Delivery, deliveries); err != nil {
		return err
	}
	if k.Delivery == "push" && (k.Producer == nil || len(k.Producer.Events) == 0 && k.Producer.Sweep == "") {
		return fmt.Errorf("a pushed kind names its producer: the events or the sweep that rebuild it")
	}
	if k.Delivery == "pull" && k.Producer != nil {
		return fmt.Errorf("a pulled kind has no producer")
	}
	if !k.ExtensionKey {
		if k.MaxLevel != "" {
			return fmt.Errorf("max_level caps extension kinds; a core kind's level is its classifier's")
		}
		return nil
	}
	if err := oneOf("max_level", k.MaxLevel, extensionLevels); err != nil {
		return fmt.Errorf("an extension kind states its ceiling: %w", err)
	}
	if k.Destination == "system_health" || k.Destination == "none" {
		return fmt.Errorf("an extension kind lands on today or review")
	}
	return nil
}

func oneOf(field, value string, allowed []string) error {
	if !slices.Contains(allowed, value) {
		return fmt.Errorf("%s %q is not one of %v", field, value, allowed)
	}
	return nil
}
