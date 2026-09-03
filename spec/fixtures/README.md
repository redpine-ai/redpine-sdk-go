# Contract fixtures

Language-neutral request/response pairs. Every SDK's contract test loads all
`*.json` here, replays `args` against the SDK method `op`, and asserts the
outgoing request equals `request` and the outcome matches `expect`.

Fields: `op`, `args` (snake_case; `filters` is a raw DSL dict), `request`
(`method`, `path`, `body` or null), `response` (`status`, `headers`, `body`),
`expect` (`error` class name or null; `field`/`value` on the parsed result).

Add a fixture whenever a wire-shape bug is fixed so all three languages pin it.

Request bodies pin the spec's image-param defaults (`imageMaxHeight: 600,
imageMaxWidth: 800, imageQuality: 75`). TS and Go hardcode these as constants
(`IMAGE_DEFAULTS` / `imageMaxHeight` etc. in `client.ts` / `client.go`); Python
inherits them from the generated model's schema defaults. If the spec's
defaults ever change, update these fixtures AND the TS/Go constants together.
