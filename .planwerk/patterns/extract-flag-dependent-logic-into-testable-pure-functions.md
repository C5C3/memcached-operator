# Pattern: Extract flag-dependent logic into testable pure functions

**Component**: cmd
**Category**: service-structure
**Applies-When**: Adding new CLI flags to cmd/main.go whose logic produces a value or side effect that can be isolated from the manager lifecycle

## Description

Flag-dependent logic that produces values consumed by ctrl.NewManager or subsequent setup calls is extracted into standalone functions (not methods, not closures) at package scope. These functions take the flag value and any dependencies as parameters, return the computed result, and contain no side effects beyond construction. This enables table-driven unit testing without starting the manager. Functions are placed between init() and main() in cmd/main.go.

## Examples

### `cmd/main.go:60`

```go
func parseWatchNamespaces(namespaces string) map[string]cache.Config {
	var result map[string]cache.Config
	for _, ns := range strings.Split(namespaces, ",") {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			continue
		}
		if result == nil {
			result = make(map[string]cache.Config)
		}
		result[ns] = cache.Config{}
	}
	return result
}
```

### `cmd/main.go:47`

```go
func buildWebhookServer(enabled bool, tlsOpts []func(*tls.Config)) webhook.Server {
	if !enabled {
		return nil
	}
	return webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})
}
```

