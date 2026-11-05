// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
)

func TestShapeCarriesAnEnumsValuesAndTheBounds(t *testing.T) {
	level := config.Define(config.Spec[string]{
		Key: "log_level", Env: "X_LEVEL", Type: config.Enum("debug", "info"), Default: config.Some("info"), Doc: "d",
	})
	shape, err := level.Shape()
	if err != nil || !slices.Equal(shape.Choices, []string{"debug", "info"}) || shape.DefaultJSON != "" {
		t.Fatalf("enum shape = %+v, %v", shape, err)
	}
	window := config.Define(config.Spec[time.Duration]{
		Key: "window", Env: "X_WINDOW", Type: config.Duration, Default: config.Some(time.Minute),
		Min: config.Some(time.Second), Max: config.Some(time.Hour), Doc: "d",
	})
	shape, err = window.Shape()
	if err != nil || shape.Min != "1s" || shape.Max != "1h0m0s" || shape.Choices != nil || shape.DefaultJSON != "" {
		t.Fatalf("duration shape = %+v, %v; a scalar's default is its DefaultText, never nanoseconds as JSON", shape, err)
	}
}

func TestShapeWritesACompoundDefaultAsJSON(t *testing.T) {
	modes := config.Define(config.Spec[map[string]string]{
		Key: "modes", Env: "X_MODES", Type: config.Map(config.String), Default: config.Some(map[string]string{"a": "b"}), Doc: "d",
	})
	shape, err := modes.Shape()
	if err != nil || shape.DefaultJSON != `{"a":"b"}` || shape.Go != reflect.TypeFor[map[string]string]() {
		t.Fatalf("map shape = %+v, %v", shape, err)
	}
}
