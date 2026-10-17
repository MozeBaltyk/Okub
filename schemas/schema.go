// Package schemas embeds the versioned Okub configuration schemas.
package schemas

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed okub.io-v1alpha1.json
var v1alpha1Document []byte

var v1alpha1 = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(v1alpha1Document, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("okub.io-v1alpha1.json", document); err != nil {
		return nil, err
	}
	return compiler.Compile("okub.io-v1alpha1.json")
})

// V1Alpha1 returns the compiled schema embedded in the Okub binary.
func V1Alpha1() (*jsonschema.Schema, error) {
	return v1alpha1()
}
