## Optional restricted live collector (Linux/macOS)

`whatsapp-bridge/cmd/monitor` is a separate opt-in binary; the stock bridge is
unchanged. It has no HTTP listener, bridge token, webhook, media downloader,
presence, typing, reaction, message-send or explicit read-receipt code.
It disables automatic history download and its history-specific receipt and
only saves new plain text in exact allowlisted chat JIDs. It ignores media
and captions, disappearing and view-once messages. It intentionally does not
guess mappings between phone and LID forms: if a chat uses a different JID, it
will not match. No allowlist means startup fails. Message text is not logged.
It uses a separate private directory for its session and message stores.

With Go 1.26, a C compiler, and verified pinned modules, build from
`whatsapp-bridge` with `go build -mod=readonly ./cmd/monitor`. A first run with
`--pair` renders a QR only on its private controlling terminal. Never relay or
record that terminal; after pairing, stop/restart without `--pair` if needed.
Launch with an absolute `--store` path in a trusted parent directory and one
or more exact `--allow-chat` flags. The read-only MCP process should point at
the resulting `messages.db` and use the same or narrower allowlist.
Disconnects stop the collector. Changing the allowlist does not purge previously
captured records; use a fresh directory if policy requires that older data
be discarded. SQLite is not encrypted by the application. Use disk encryption,
exclude the directory from backups, and keep it off shared/cloud storage.

“Read-only” here means no application-level messaging or read-state actions.
The unofficial WhatsApp device still registers, syncs protocol state, and
can emit required protocol acknowledgements and delivery/retry receipts
through the linked-device library. Its session database holds account
credentials and may retain protocol metadata beyond the chat allowlist.
The collector does not prevent WhatsApp transporting other chats to the linked
device; only the app's message database is scoped. Pairing, delivery behavior,
and long-running operation are not validated by synthetic tests.
