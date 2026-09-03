# redpine (Go)

Go client for the Redpine search API. Go 1.24+.

```bash
go get github.com/redpine-ai/redpine-sdk-go@latest
```

```go
import redpine "github.com/redpine-ai/redpine-sdk-go"

client, err := redpine.New() // reads REDPINE_API_KEY
r, err := client.Search(ctx, "crispr delivery", redpine.SearchOptions{Collections: []string{"corpus"}})
```

Docs: https://docs.redpine.ai/docs/sdks
