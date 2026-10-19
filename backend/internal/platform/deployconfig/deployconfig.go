// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package deployconfig reads what an installation is created with: the
// workspace, the first administrator and the seeds of margince.yaml. Every
// other key of the file is a setting config.Load reads. A seed is consumed
// once, when the workspace is created, and the database is authoritative
// afterwards. Decoding is strict (an unknown key inside these roots is a boot
// error) and the admin password arrives only as a reference.
package deployconfig

import (
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// Bootstrap is the four roots of margince.yaml this package reads. Every one
// beyond `version` is optional: a missing file boots an already-bootstrapped
// installation, and bootstrapping an empty database needs `workspace` and
// `bootstrap_admin`.
type Bootstrap struct {
	Version        int             `yaml:"version"`
	Workspace      Workspace       `yaml:"workspace"`
	BootstrapAdmin *BootstrapAdmin `yaml:"bootstrap_admin"`
	Seeds          Seeds           `yaml:"seeds"`
}

// Workspace names the installation's singleton workspace — the tenant every
// row is filed under, and what the schema has always called it. Consumed only
// when the workspace is created; it never reconciles into an existing
// installation.
//
// It was spelled `company` until the schema and the config disagreed
// loudly enough to notice: `workspace` is the table, `workspace_id` is the
// column on every tenant row, and an operator reading both had to know that
// the two words meant one thing. Nothing here is the CRM's company record.
type Workspace struct {
	Name         string `yaml:"name"`
	BaseCurrency string `yaml:"base_currency"`
	BaseLanguage string `yaml:"base_language"`
	Timezone     string `yaml:"timezone"`
	// Country is the jurisdiction the installation runs under, as a lower-case
	// ISO 3166-1 alpha-2 code naming a compiled-in pack. Empty leaves it unset.
	// Seeded once, also into an existing installation that has none stored;
	// after that Settings → Jurisdiction changes it, never this file.
	Country string `yaml:"country"`
}

// BootstrapAdmin identifies the first administrator. The password is a
// reference so the secret can be deleted after first boot — once the
// workspace exists this whole section may be removed.
type BootstrapAdmin struct {
	Email       string `yaml:"email"`
	DisplayName string `yaml:"display_name"`
	// Password is the reference form: ${file:...} or ${env:...}.
	Password Secret `yaml:"password"`
	// PasswordFile is the original spelling, still honoured so an existing
	// deployment boots unchanged. It is a bare path rather than a reference,
	// which is why it is not a Secret: it names a file and cannot name the
	// environment. Prefer `password`.
	PasswordFile string `yaml:"password_file"`
}

// ResolvePassword reads the bootstrap admin's password from whichever source
// the deployment named. Called only on the bootstrap path — an
// already-bootstrapped installation never needs (or reads) the secret.
//
// The reference wins when both are present, because it is the form that can
// name the environment and the one an operator migrating forward would have
// added deliberately.
func (b BootstrapAdmin) ResolvePassword(lookup config.Lookup) (string, error) {
	var pw string
	var err error
	switch {
	case b.Password.Configured():
		pw, err = b.Password.withField("bootstrap_admin.password").Resolve(lookup)
	case b.PasswordFile != "":
		pw, err = Secret{kind: secretFromFile, arg: b.PasswordFile, field: "bootstrap_admin.password_file"}.Resolve(lookup)
	default:
		return "", errors.New("deployconfig: bootstrap_admin names no password — set bootstrap_admin.password to ${file:/run/secrets/admin-password} or ${env:MARGINCE_ADMIN_PASSWORD}")
	}
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", b.passwordRef().Missing()
	}
	// The floor is the product's, not this file's: a bootstrap that planted a
	// password the change route would refuse would strand the account it
	// created.
	if len([]rune(pw)) < 12 {
		return "", errors.New("deployconfig: the bootstrap_admin password must be at least 12 characters")
	}
	return pw, nil
}

// passwordRef is whichever of the two spellings this deployment used, so a
// failure names the key the operator actually wrote.
func (b BootstrapAdmin) passwordRef() Secret {
	if b.Password.Configured() {
		return b.Password.withField("bootstrap_admin.password")
	}
	return Secret{kind: secretFromFile, arg: b.PasswordFile, field: "bootstrap_admin.password_file"}
}

// RetentionSeed selects the retention POSTURE a fresh installation is
// bootstrapped into. It does not select the policy rows: bootstrap plants the
// same built-in rows under either posture. So an installation that must keep
// everything still SEES the ladder it is not running.
//
// The key exists because bootstrap plants the rows and the nightly pass can fire
// before an admin first logs in. A deployment under a contractual
// keep-everything obligation closes that window here rather than racing it in the
// UI.
type RetentionSeed struct {
	// DefaultPolicy is `standard` or `retain_only`. Empty means standard, which
	// is the historical behaviour byte for byte.
	DefaultPolicy string `yaml:"default_policy"`
}

// The two postures seeds.retention.default_policy admits.
const (
	// RetentionStandardPosture plants the built-in rows with destruction enabled:
	// storage limitation (Art. 5(1)(e)) as the out-of-the-box posture.
	RetentionStandardPosture = "standard"
	// RetentionRetainOnlyPosture plants the same rows and turns the retain-only
	// posture on, so nothing is anonymized or erased however over-age it becomes.
	RetentionRetainOnlyPosture = "retain_only"
)

// RetainOnly reports whether the configured posture suppresses every destructive
// retention action. An absent block is the standard posture, so a minimal file
// behaves exactly like the historical bootstrap.
func (s Seeds) RetainOnly() bool {
	return s.Retention != nil && s.Retention.DefaultPolicy == RetentionRetainOnlyPosture
}

// PipelineSeed configures the default pipeline's open stages. Won/Lost
// terminal stages are appended by the deals module — stage semantics are
// a module invariant, not an operator choice.
type PipelineSeed struct {
	Name   string          `yaml:"name"`
	Stages []PipelineStage `yaml:"stages"`
}

// PipelineStage is one configured open stage: display name + win
// probability.
type PipelineStage struct {
	Name        string `yaml:"name"`
	Probability int    `yaml:"probability"`
}

// ConsentPurpose seeds one row of the consent purpose catalog.
type ConsentPurpose struct {
	Key         string `yaml:"key"`
	Label       string `yaml:"label"`
	DoubleOptIn bool   `yaml:"double_opt_in"`
}

func (c Bootstrap) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("deployconfig: unsupported version %d (this build supports version 1)", c.Version)
	}
	if c.Workspace.Timezone != "" {
		if _, err := values.ParseTimezone(c.Workspace.Timezone); err != nil {
			return fmt.Errorf("deployconfig: workspace.timezone: %w", err)
		}
	}
	if cur := c.Workspace.BaseCurrency; cur != "" && !values.ValidCurrency(cur) {
		return fmt.Errorf("deployconfig: workspace.base_currency %q is not a 3-letter ISO 4217 code", cur)
	}
	if code := c.Workspace.Country; code != "" {
		if err := jurisdiction.Code(code).Validate(); err != nil {
			return fmt.Errorf("deployconfig: workspace.country: %w", err)
		}
	}
	if lang := c.Workspace.BaseLanguage; lang != "" && !textlang.Known(lang) {
		return fmt.Errorf("deployconfig: workspace.base_language %q is not a language this build speaks (en, de, vi)", lang)
	}
	if c.BootstrapAdmin != nil {
		if err := c.BootstrapAdmin.validate(); err != nil {
			return err
		}
	}
	return c.Seeds.validate()
}

func (b BootstrapAdmin) validate() error {
	if _, err := values.ParseEmail(b.Email); err != nil {
		return fmt.Errorf("deployconfig: bootstrap_admin.email: %w", err)
	}
	if b.DisplayName == "" {
		return errors.New("deployconfig: bootstrap_admin.display_name is required")
	}
	// EITHER spelling, and the same sentence ResolvePassword gives when neither
	// is set. Checking password_file alone refused a deployment that wrote the
	// key this file's own doc comment says to prefer, and told the operator to
	// use the legacy one instead — so the natural read was that the reference
	// form is unsupported, at first boot, with nothing to correct it.
	//
	// It also made the validator and the resolver two different contracts for
	// one field: Parse refused a document ResolvePassword handles perfectly.
	if !b.Password.Configured() && b.PasswordFile == "" {
		return errors.New("deployconfig: bootstrap_admin names no password — set bootstrap_admin.password to ${file:/run/secrets/admin-password} or ${env:MARGINCE_ADMIN_PASSWORD}")
	}
	return nil
}

func (s Seeds) validate() error {
	if s.Pipeline != nil {
		if err := s.Pipeline.validate(); err != nil {
			return err
		}
	}
	if s.Retention != nil {
		if err := s.Retention.validate(); err != nil {
			return err
		}
	}
	seenKeys := map[string]bool{}
	for _, p := range s.ConsentPurposes {
		if p.Key == "" || p.Label == "" {
			return errors.New("deployconfig: seeds.consent_purposes entries need key and label")
		}
		if seenKeys[p.Key] {
			return fmt.Errorf("deployconfig: seeds.consent_purposes key %q is listed twice", p.Key)
		}
		seenKeys[p.Key] = true
	}
	return nil
}

// validate refuses an unrecognized posture loudly. A typo here would otherwise
// fall back to `standard` in silence — which for the one deployment that set the
// key on purpose is the exact failure it was set to prevent.
func (r RetentionSeed) validate() error {
	switch r.DefaultPolicy {
	case "", RetentionStandardPosture, RetentionRetainOnlyPosture:
		return nil
	default:
		return fmt.Errorf("deployconfig: seeds.retention.default_policy %q is not %s or %s",
			r.DefaultPolicy, RetentionStandardPosture, RetentionRetainOnlyPosture)
	}
}

func (p PipelineSeed) validate() error {
	if p.Name == "" {
		return errors.New("deployconfig: seeds.pipeline.name is required")
	}
	if len(p.Stages) == 0 {
		return errors.New("deployconfig: seeds.pipeline.stages must name at least one open stage")
	}
	seen := map[string]bool{}
	for _, st := range p.Stages {
		if st.Name == "" {
			return errors.New("deployconfig: seeds.pipeline.stages entries need a name")
		}
		if seen[st.Name] {
			return fmt.Errorf("deployconfig: seeds.pipeline stage %q is listed twice", st.Name)
		}
		seen[st.Name] = true
		if st.Probability < 0 || st.Probability > 100 {
			return fmt.Errorf("deployconfig: seeds.pipeline stage %q probability %d is outside 0–100", st.Name, st.Probability)
		}
	}
	return nil
}
