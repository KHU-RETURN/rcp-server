//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	err := entc.Generate("./internal/schema", &gen.Config{
		Target: "./ent",
	})
	if err != nil {
		log.Fatal(err)
	}
}
