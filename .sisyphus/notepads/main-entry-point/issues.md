# Issues

## Go embed: `..` not allowed in patterns

The task specified `//go:embed ../../web/dist` in `cmd/phosche/embed.go`, but Go prohibits
`..` in embed path patterns. Workaround: placed `embed.go` at project root with 
`//go:embed web/dist` and used a shared `internal/app/run.go` for both entry points.

## No `fs.Stat` check for embed.FS availability

The SPA handler uses `fs.Stat` to check file existence before falling back to `index.html`.
This works for both `embed.FS` and filesystem `os.DirFS`.
