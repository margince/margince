// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// aiConfig is this package's settings as a program loads them from environ
// alone: no flag, and no file beside the test.
func aiConfig(t *testing.T, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatalf("the ai settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the ai settings from %v: %v", environ, err)
	}
	return cfg
}

// storedOver is the settings store a program builds over pool, and the
// configuration that reads and saves through it under environ's locks.
func storedOver(t *testing.T, pool *pgxpool.Pool, environ ...string) (*settings.Store, *config.Config) {
	t.Helper()
	cfg := aiConfig(t, environ...)
	store := settings.New(pool, settings.NewRegistry(append(Definitions(), cfg.StoredDefinitions()...)...))
	return store, cfg.WithDatabase(store)
}

// storedRow is the row a program registers for key.
func storedRow(t *testing.T, key string) settings.Definition {
	t.Helper()
	for _, def := range aiConfig(t).StoredDefinitions() {
		if def.Key() == key {
			return def
		}
	}
	t.Fatalf("no stored ai setting is keyed %s", key)
	return nil
}

// A boot value is held to the bar a save is. One outside a setting's bounds
// fails the boot, rather than lock a value no admin could save.
func TestABootValueASaveWouldRefuseFailsTheLoad(t *testing.T) {
	refused := map[string]string{
		"a budget of no tokens per seat":   `MARGINCE_AI_BUDGET={"tokens_per_full_user":0,"company_monthly_tokens":null}`,
		"a budget past the ceiling":        `MARGINCE_AI_BUDGET={"tokens_per_full_user":1,"company_monthly_tokens":1000000000001}`,
		"a budget missing a field":         `MARGINCE_AI_BUDGET={"tokens_per_full_user":5}`,
		"an attempt timeout below bounds":  `MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"attempt_timeout_ms":1}}`,
		"a decision timeout past bounds":   `MARGINCE_AI_TASK_OVERRIDES={"capture_confidentiality_verdict":{"decision_timeout_ms":600000}}`,
		"a thinking level no adapter maps": `MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"extreme"}}`,
		"a profile that binds no tier":     `MARGINCE_AI_ROUTING_BINDING={"profile":"eu_hosted","tiers":{}}`,
		"a price switch with a stray key":  `MARGINCE_AI_PRICE_SYNC={"auto_sync":true,"hourly":true}`,
	}
	for name, env := range refused {
		t.Run(name, func(t *testing.T) {
			reg, err := config.NewRegistry(ConfigSettings())
			if err != nil {
				t.Fatal(err)
			}
			_, err = config.Load(reg, config.Sources{
				Environ:  []string{env},
				ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
			})
			if err == nil {
				t.Errorf("%s loaded; a save would refuse it", env)
			}
		})
	}
}

// Each value a save accepts is accepted from the environment too, and locks.
func TestABootValueASaveWouldAcceptLocksItsSetting(t *testing.T) {
	cfg := aiConfig(t,
		`MARGINCE_AI_BUDGET={"tokens_per_full_user":5,"company_monthly_tokens":null}`,
		`MARGINCE_AI_TASK_OVERRIDES={"capture_confidentiality_verdict":{"decision_timeout_ms":30000,"attempt_timeout_ms":10000}}`,
		`MARGINCE_AI_PRICE_SYNC={"auto_sync":false}`,
	)
	for _, key := range []string{BudgetKey, TaskOverridesKey, PriceSyncKey} {
		if _, locked := cfg.Locked(key); !locked {
			t.Errorf("%s is not locked by the value the environment sets", key)
		}
	}
	if _, locked := cfg.Locked(RoutingKey); locked {
		t.Error("ai.routing is locked with no source setting it")
	}
}

// The census reads each key as a literal, and callers spell it through a
// constant; the two must stay one key.
func TestEachKeyConstantNamesItsSetting(t *testing.T) {
	for constant, s := range map[string]config.Any{
		RoutingKey: Routing, TaskOverridesKey: TaskOverridesSetting, BudgetKey: BudgetSettings, PriceSyncKey: PriceSyncSettings,
	} {
		if s.Key() != constant {
			t.Errorf("the constant %q names a setting keyed %q", constant, s.Key())
		}
	}
}

// A budget in margince.yaml and the same budget in the environment load alike,
// so neither source admits a document the other refuses.
func TestABudgetLoadsAlikeFromTheFileAndTheEnvironment(t *testing.T) {
	for _, doc := range []string{
		`{"tokens_per_full_user":5,"company_monthly_tokens":777}`,
		`{"tokens_per_full_user":5}`,
		`{"tokens_per_full_user":0,"company_monthly_tokens":null}`,
	} {
		fromFile, fileErr := loadBudget(t, "ai:\n  budget: "+doc+"\n", nil)
		fromEnv, envErr := loadBudget(t, "", []string{"MARGINCE_AI_BUDGET=" + doc})
		if (fileErr == nil) != (envErr == nil) || !reflect.DeepEqual(fromFile, fromEnv) {
			t.Errorf("%s: file %+v %v, environment %+v %v; want one answer", doc, fromFile, fileErr, fromEnv, envErr)
		}
	}
}

func loadBudget(t *testing.T, file string, environ []string) (BudgetConfig, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "margince.yaml")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{Args: []string{"--config", path}, Environ: environ})
	if err != nil {
		return BudgetConfig{}, err
	}
	budget, ok := config.BootVal(cfg, BudgetSettings)
	if !ok {
		t.Fatalf("the budget %q / %v set no boot value", file, environ)
	}
	return budget, nil
}

// Overrides in margince.yaml are spelled as the API spells them.
func TestTaskOverridesLoadFromTheFileByTheirAPINames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "margince.yaml")
	file := "ai:\n  task_overrides:\n    capture_classify: {thinking: low, attempt_timeout_ms: 10000}\n"
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(reg, config.Sources{Args: []string{"--config", path}})
	if err != nil {
		t.Fatalf("loading %q: %v", file, err)
	}
	got, ok := config.BootVal(cfg, TaskOverridesSetting)
	if !ok {
		t.Fatal("the file set no task overrides")
	}
	if want := (TaskOverride{Thinking: "low", AttemptTimeoutMs: 10000}); got[TaskCaptureClassify] != want {
		t.Errorf("capture_classify loaded as %+v, want %+v", got[TaskCaptureClassify], want)
	}
}
