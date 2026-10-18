# Chat Image Extension

This directory contains the complete user-facing chat and image generation feature.
Feature-specific frontend code remains here so upstream updates can be merged with a
small, explicit integration surface.

## Billing and authentication

- The logged-in user's existing JWT is used only for API key management.
- `Chat` is automatically created in the `gpt-pro` group.
- `Draw` is automatically created in the `gpt-Image-2` group.
- Legacy `__chat_image_chat__` and `__chat_image_draw__` keys are renamed in place.
- Model and generation requests use those keys through the native `/v1` gateway, so
  the existing group multipliers, balance deduction, quotas, and usage logs apply.
- Generated keys are held in memory and are not written to localStorage.
- Image requests use the synchronous Images endpoints with URL responses and the same
  fixed image key.

Users must be allowed to bind API keys to both groups. Group name matching is
case-insensitive, but the names themselves must otherwise match.

## Persistent history

- Conversations and messages are stored in PostgreSQL through `/api/v1/chat-image`.
- Generated images and files remain in the upstream channel/object store. The database
  stores only their HTTP(S) or root-relative access path and optional display metadata.
- Binary data and `data:` URLs are never persisted. A base64-only image response can be
  displayed for the current page session but is unavailable after reload.
- Message writes use a client UUID and a database uniqueness constraint, making retries
  idempotent.
- Conversation lists are paginated and searched on the server; messages load on demand.
- Light and dark themes use the existing application `.dark` theme state.

## Prompt library

- The extension backend serves a bundled 301-item snapshot adapted from the sibling
  `chatgpt2api` prompt library through `GET /api/v1/chat-image/prompts`.
- Search, source/category filtering, progressive loading, preview fallback, and prompt
  image settings are implemented entirely inside this extension directory.
- Preview images stay on their upstream public host; Sub2API stores no prompt assets.
- The extension never calls `chatgpt2api` or `/backend-api/conversation` at runtime.

## Upstream merge points

The frontend has only two integration points outside this directory:

1. `src/router/index.ts` imports and spreads `chatImageRoutes`.
2. `src/components/layout/AppSidebar.vue` imports `ChatImageNavIcon` and adds the
   `/chat-image` navigation item.

The backend implementation is isolated in `backend/internal/extensions/chatimage`. Native
server integration is limited to passing the existing `*sql.DB` into the router and
registering that package from `backend/internal/server/router.go`. Schema changes live in
`backend/migrations/239_chat_image_persistence.sql` and do not modify generated Ent code.

When updating Sub2API, retain those small integration points, the two extension directories,
and migration 239.
