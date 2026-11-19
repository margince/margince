---
layers:
  - jp/jp.md
---

# Japanese messaging policy

The layers that `gen-messaging-rules` compiles into `messagingrules_gen.go`. The region shares no
law, so the pack lists one layer, the country layer in `jp/jp.md`.

Edit the layer, then run `go generate` in `extensions/apac`, or `make -C backend gen`. The drift lane
fails a generated file that no longer matches its sources.
