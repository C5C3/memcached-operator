# Operator Logging

Reference documentation for the operator's log output: the format and level the
manager logs at by default, the five `--zap-*` flags that change them, and how
to set them through Helm, Kustomize, and a local run.

**Source**: `cmd/main.go`, `charts/memcached-operator/templates/deployment.yaml`

## Production Defaults

The manager starts its logger in controller-runtime's production mode. Without
a logging flag it writes:

- one JSON object per line, to stderr
- entries at info level and above
- a stack trace on entries at error level and above
- RFC 3339 timestamps in the `ts` field
- sampled output: per second, the first 100 entries with the same level and
  message, then every 100th
- DPanic-level entries as ordinary log lines; the process does not panic

A startup line reads:

```json
{"level":"info","ts":"2026-11-22T14:47:04Z","logger":"setup","msg":"watching all namespaces"}
```

`bindLogFlags` in `cmd/main.go` registers the flags on an options struct that
starts in production mode. `--zap-devel` switches to development mode, which
writes tab-separated console lines at debug level (tabs shown as two spaces):

```text
2026-11-22T14:47:04Z  INFO  setup  watching all namespaces
```

---

## Flags

The flags come from controller-runtime's `zap.Options.BindFlags`. An explicit
flag overrides the default of the mode: `--zap-devel --zap-log-level=info`
writes console lines at info level.

| Flag                     | Accepted values                                                             | Production default | Development default (`--zap-devel`) |
|--------------------------|-----------------------------------------------------------------------------|--------------------|-------------------------------------|
| `--zap-devel`            | `true`, `false`                                                             | `false`            | `true`                              |
| `--zap-encoder`          | `json`, `console`                                                           | `json`             | `console`                           |
| `--zap-log-level`        | `debug`, `info`, `error`, `panic` in any case, or an integer greater than 0 | `info`             | `debug`                             |
| `--zap-stacktrace-level` | `info`, `error`, `panic`                                                    | `error`            | `warn`                              |
| `--zap-time-encoding`    | `epoch`, `millis`, `nanos`, `iso8601`, `rfc3339`, `rfc3339nano`             | `rfc3339`          | `rfc3339`                           |

The help text of `--zap-time-encoding` names `epoch` as the default and lists
`nano`. Both are wrong for controller-runtime v0.23.1: the default is
`rfc3339`, and the flag accepts `nanos` and rejects `nano`. The development
default of `--zap-stacktrace-level`, `warn`, cannot be set with the flag.

### Debug and Integer Levels

`debug` is the same as `1`. An integer `N` enables every entry logged at
verbosity `V(N)` and below. The operator's own code logs below info in one
place: `internal/controller/memcached_controller.go:190` logs "Skipping
Deployment not controlled by this Memcached" at `V(1)`. controller-runtime
logs at `V(1)` as well, and integer levels above 1 add its more verbose
entries (it logs at `V(2)`, `V(4)`, `V(5)`, and `V(7)`).

### Sampling

Production mode samples entries: per second, the first 100 entries with the
same level and message are written, then every 100th. Sampling is off at an
integer level of 2 and above, and in development mode.

### Invalid Values

The manager parses its flags before it contacts the API server. An invalid
value prints the error and the usage text and exits with status 2:

```text
invalid value "warn" for flag -zap-log-level: invalid log level "warn"
```

In a cluster the pod then restarts in a crash loop.

---

## Helm

The chart passes the log level through one value, `logLevel`, rendered as
`--zap-log-level`. The default `info` is rendered explicitly, so the level is
visible in the pod spec. Set it in a values file (`logLevel: debug`) or on the
command line, together with the values the release already uses:

```bash
helm upgrade memcached-operator oci://ghcr.io/c5c3/charts/memcached-operator \
  --version <version> \
  --set logLevel=debug
```

| `logLevel`                                | Rendered                                                |
|-------------------------------------------|---------------------------------------------------------|
| `info` (default)                          | `--zap-log-level=info`                                  |
| `debug`, `error`, `panic`                 | `--zap-log-level=<value>`                               |
| `2` as a number or a string               | `--zap-log-level=2`                                     |
| `""` or `null`                            | No `--zap-log-level` argument; the manager logs at info |
| Anything else: `warn`, `DEBUG`, `0`, `-1` | Rendering fails                                         |

An invalid value fails at render time, before anything reaches the cluster:

```text
logLevel must be one of debug, info, error, panic, or an integer greater than 0, got "warn"
```

The chart compares case-sensitively, although the flag does not, so values
files carry one spelling. The chart has no value for `--zap-devel`,
`--zap-encoder`, `--zap-stacktrace-level`, or `--zap-time-encoding`; to pass
one of them, use a Helm post-renderer.

---

## Kustomize

The manifests under `config/` pass no logging flag, so a Kustomize deployment
runs with the production defaults. To add a flag, use a JSON patch in a
kustomization whose `resources` lists `config/default`. A strategic-merge
patch replaces the whole argument list (as
`config/default/manager_metrics_patch.yaml` does), so append to it instead:

```yaml
patches:
  - target:
      group: apps
      version: v1
      kind: Deployment
      name: memcached-operator-controller-manager
    patch: |-
      - op: add
        path: /spec/template/spec/containers/0/args/-
        value: --zap-log-level=debug
```

Set `value: --zap-devel` to get the console output at debug level back.

---

## Local Development

`make run` starts the manager with `--zap-devel`, so a local run writes
console lines at debug level. To try another setting, run the manager directly
with the flags, for example production mode at verbosity 2:

```bash
go run ./cmd/main.go --zap-log-level=2
```
