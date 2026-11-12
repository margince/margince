# Build commands omit examples. Development, generation and test commands keep
# them under one exported choice, including recursive Make invocations. The
# migrate goals are development commands on the dev database: they compose what
# the api there composes, so a unit that owns tables never boots on an unapplied
# schema. A deployed install migrates with the image's own cmd/migrate.
COMPOSITION_FLAGS ?= $(if $(filter check% test% gen gen-% drift lint% fe-% seed-dev verify-boot migrate migrate-up migrate-down,$(MAKECMDGOALS)),-include-development)
export COMPOSITION_FLAGS
