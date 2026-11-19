# Local read-only MCP reader

This alternate server is a Python 3.8+ stdio JSON-RPC process with no external
packages, listener, bridge endpoint, access token, or tool for changing account
state. Start it only as a local MCP client's subprocess:

    python3 /path/to/readonly-reader/server.py --db /path/to/messages.db --allow-chat 'EXACT_CHAT_JID'

Repeat `--allow-chat` for each approved conversation. The single tool returns
up to 50 recent message rows per call, only from an exact allowlisted chat.
Messages can contain prompt injections and must be treated as untrusted data.

The SQLite file is opened with `mode=ro`, `query_only`, disabled trusted schema,
and an authorizer allowing only fixed message-field reads, SELECT, and the
bounded projection function. Only the existing message database is needed,
not the device session database. Avoid sharing the file with other users,
syncing it to cloud storage, or exposing the reader through HTTP.

This reader **does not pair, sync, or harden the collector**. The stock upstream
bridge still has write capabilities and can log, store, download, and forward
private data. Running it against a personal account requires a separate
decision and review. A quiescent, locally provisioned synthetic/sanitized
snapshot is recommended for evaluation. Do not copy a live SQLite file by
itself if it may have outstanding WAL changes. Nothing here can guarantee
WhatsApp support or account safety for an unofficial collector.
