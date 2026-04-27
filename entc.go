//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	err := entc.Generate("./internal/schema", &gen.Config{
		Target:   "./ent",
		Package:  "github.com/KHU-RETURN/rcp-server/ent",
		Features: []gen.Feature{gen.FeatureUpsert},
	})
	if err != nil {
		log.Fatal(err)
	}
}
