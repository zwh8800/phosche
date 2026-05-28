# T20 Learnings

## axios + MSW compatibility
- axios's default Node.js `http` adapter conflicts with MSW v2, causing "Invalid URL" errors
- Solution: Use `adapter: 'fetch'` in axios config — works with MSW in both Node (test) and browser (prod)
- MSW handlers must match full URLs (not relative paths) when axios uses absolute baseURL

## Environment-aware baseURL
- Used `typeof window === 'undefined'` to detect Node vs browser
- Node (test): `http://localhost:8080/api`
- Browser (vite dev): `/api` (vite proxy handles forwarding to backend)

## Tailwind CSS v4
- No `tailwind.config.js` needed
- Just import `'tailwindcss'` in CSS and add `@tailwindcss/vite` plugin
- This project uses Tailwind v4.3.0

## vitest v4
- Config file: `vitest.config.ts` with `import { defineConfig } from 'vitest/config'`
- `globals: true` for describe/it/expect without imports
