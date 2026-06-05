# Presto contract tests

These tests validate the agent backend contract without depending on unfinished core package symbols.

Run all Presto tests, including these contract fixtures:

```sh
cd /Users/sunnychen/Dev/JobAgent/Agents/presto
go test ./...
```

Run live HTTP smoke validation when the Presto server is available:

```sh
cd /Users/sunnychen/Dev/JobAgent/Agents/presto
PRESTO_HTTP_BASE_URL=http://127.0.0.1:8080 go test ./tests -run TestHTTPAPIMultiTurnSessionIfConfigured -count=1
```

Optional live HTTP settings:

- `PRESTO_CHAT_PATH`: chat path, default `/v1/chat/completions`
- `PRESTO_MODEL`: model name, default `presto-contract`
- `PRESTO_API_KEY`: bearer token, omitted by default
- `PRESTO_SESSION_HEADER`: session header, default `X-Session-ID`
- `PRESTO_SESSION_FIELD`: JSON session field, default `session_id`; set empty to omit it
