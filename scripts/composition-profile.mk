# Build commands omit examples. Development, generation and test commands keep
# them under one exported choice, including recursive Make invocations.
COMPOSITION_FLAGS ?= $(if $(filter check% test% gen gen-% drift lint% fe-% seed-dev verify-boot,$(MAKECMDGOALS)),-include-development)
export COMPOSITION_FLAGS
