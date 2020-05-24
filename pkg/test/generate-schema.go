//+build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/alecthomas/jsonschema"
	"github.com/csweichel/dazzle/pkg/test"
)

func main() {
	var root []test.Spec
	schema := jsonschema.Reflect(&root)

	fc, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(fc))
}
