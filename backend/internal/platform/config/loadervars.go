// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "slices"

// LoaderVar is a variable Load reads itself, before any setting: the posture,
// where the files are, and whether to trace. Flag is "" for the posture, which
// only the environment sets.
type LoaderVar struct{ Env, Flag, Doc string }

// loaderVars lists them. The reserved names and the flag usage are built
// from it, and the generated .env.example lists it.
var loaderVars = []LoaderVar{
	{Env: envPosture, Doc: "The posture: dev or test runs without a license, and unset or any other value is production."},
	{Env: envConfig, Flag: "config", Doc: "Base YAML file; margince.yaml when unset."},
	{Env: envConfigFile, Flag: "config-file", Doc: "YAML file layered over the base file."},
	{Env: envEnvFile, Flag: "env-file", Doc: ".env file to read."},
	{Env: envTrace, Flag: "config-trace", Doc: "Log every configuration source and the origin of each setting."},
}

// LoaderVars lists the variables Load reads before any setting.
func LoaderVars() []LoaderVar { return slices.Clone(loaderVars) }

// loaderDoc is the usage text of one of the loader's own flags.
func loaderDoc(flag string) string {
	at := slices.IndexFunc(loaderVars, func(v LoaderVar) bool { return v.Flag == flag })
	if at < 0 {
		return ""
	}
	return loaderVars[at].Doc
}

// reservedNames are read by Load itself, so a setting with one of them would
// be read before it exists.
func reservedNames() map[string]bool {
	out := map[string]bool{}
	for _, v := range loaderVars {
		out[v.Env] = true
		if v.Flag != "" {
			out[v.Flag] = true
		}
	}
	return out
}
