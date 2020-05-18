//+build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/csweichel/dazzle/pkg/test"
	"github.com/alecthomas/jsonschema"
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
