# Black-box API baseline

A suite that talks only HTTP to a running entity-service and records
normalized results, so two builds of the service can be compared.

```
# build tree A, start a throwaway PostgreSQL 18, migrate it with A's migrations,
# seed fictional data, run every scenario, write results, tear everything down
./run.sh run --tree /path/to/checkout-A --out /tmp/a.json
./run.sh run --tree /path/to/checkout-B --out /tmp/b.json

# per-call differences, exit code 1 when any
./run.sh compare /tmp/a.json /tmp/b.json
```

`--tree` is a repository checkout or its `entity-service` directory. Needs bash,
go, curl and the PostgreSQL 18 binaries (`PG_BIN`, default
`/opt/homebrew/opt/postgresql@18/bin`). A run takes about four minutes, most of
it applying roughly 400 migrations. `--keep` keeps the work directory, `--hold`
leaves the server up for manual curl (see `apibaseline token`).

- `seed.sql`: fictional accounts, projects, users, products, SLA policies, a
  service-request catalog. `fixture-0031.sql`: reference rows a migration expects.
- `s1_cases.go` ... `s5_misc.go`: the scenarios, in execution order. Each is a
  named step with a one-line statement of what the baseline is expected to do.
- Identity is the gateway's: a signed user token and a client assertion, minted
  here with a throwaway key whose key set the server fetches at startup.
- Normalization masks ids, timestamps, generated numbers, tokens and
  time-derived numbers; seeded ids get readable names. Rows the server returns
  in no defined order are sorted or projected inside the scenario that needs it.
- Migrations that cannot apply to an empty database are retried after the
  legacy phase, then skipped and listed next to the results (set
  `APIBASELINE_STRICT=1` to make that fatal).
