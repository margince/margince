---
layers:
  - us/us.md
---

# United States messaging policy

The layers that `gen-messaging-rules` compiles into `messagingrules_gen.go`. The pack lists one
layer, the country layer in `us/us.md`.

Edit the layer, then run `go generate` in `extensions/us`, or `make -C backend gen`. The drift lane
fails a generated file that no longer matches its sources.
