// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
)

// SettingsFile names the file a unit's Settings FS holds at its root: a JSON
// object of setting key to value.
const SettingsFile = "settings.json"

// settingKeyGrammar is the part of a stored key after the namespace. It is the
// right-hand half of the `setting_key_check` constraint, so a key accepted here
// is a key the table stores.
var settingKeyGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// SettingKey is the stored key of one of the unit's settings. The `ext_`
// namespace keeps it apart from core's `<module>.<name>` keys and from every
// other unit's; it is the namespace the unit's tables and role already use.
func (n Name) SettingKey(key string) (string, error) {
	namespace, err := n.Namespace()
	if err != nil {
		return "", err
	}
	if !settingKeyGrammar.MatchString(key) {
		return "", fmt.Errorf("setting key %q is not valid: use lower-case letters, digits and underscores, starting with a letter", key)
	}
	return namespace + "." + key, nil
}

// SettingsFrom reads the declared settings. It refuses a missing file, a
// document that is not one JSON object and a key core cannot store. A bad
// declaration then stops the seed instead of being half applied.
func SettingsFrom(fsys fs.FS) (map[string]json.RawMessage, error) {
	raw, err := fs.ReadFile(fsys, SettingsFile)
	if err != nil {
		return nil, fmt.Errorf("a unit that declares Settings must hold %s at its root: %w", SettingsFile, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var values map[string]json.RawMessage
	if err := dec.Decode(&values); err != nil {
		return nil, fmt.Errorf("%s must be one JSON object of setting key to value: %w", SettingsFile, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s holds content after its JSON object", SettingsFile)
	}
	for key := range values {
		if !settingKeyGrammar.MatchString(key) {
			return nil, fmt.Errorf("setting key %q is not valid: use lower-case letters, digits and underscores, starting with a letter", key)
		}
	}
	return values, nil
}
