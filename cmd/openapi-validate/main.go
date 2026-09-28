package main

import (
	"context"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	if err := validateOpenAPI("api/openapi.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("OpenAPI contract is valid: api/openapi.yaml")
}

func validateOpenAPI(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OpenAPI contract: %w", err)
	}
	return validateOpenAPIData(data)
}

func validateOpenAPIData(data []byte) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	document, err := loader.LoadFromData(data)
	if err != nil {
		return fmt.Errorf("load OpenAPI contract: %w", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		return fmt.Errorf("validate OpenAPI contract: %w", err)
	}
	return nil
}
