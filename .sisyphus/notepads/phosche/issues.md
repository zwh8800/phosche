# Issues - Phosche

## Known Gotchas
- fsnotify does NOT support recursive watching natively → must implement walk+add
- HEIC decoding may require CGo/libheif → need to test on Docker Linux
- ES 8.x defaults to HTTPS with self-signed certs → config option for insecure_skip_verify
- Ollama chat API uses images field in messages, OpenAI uses image_url in content array
- ES mapping versions: check _meta.version, warn on mismatch, no auto-migration

## Go 1.26 Compatibility
- Short variable declarations (`x := T{...}`) do NOT allow self-referencing closures inside the struct literal. Use `var x *T; x = &T{...}` instead. This affected mock indexer construction in pipeline tests where `indexFn` closures needed to reference the outer `indexer` variable.
