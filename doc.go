// Package redpine is the Go client for the Redpine search API.
//
//	client, err := redpine.New()            // reads REDPINE_API_KEY
//	r, err := client.Search(ctx, "crispr", redpine.SearchOptions{
//	    Collections: []string{"corpus"},
//	    Filters:     redpine.F("issn").Eq("1664-302X").Or(redpine.F("issn").Eq("1932-6203")),
//	})
package redpine
