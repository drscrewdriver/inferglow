package server

import (
	"testing"
)

func TestValidator_Initialized(t *testing.T) {
	if validate == nil {
		t.Fatal("validate instance is nil")
	}
}

type testStruct struct {
	Name string `validate:"required"`
}

func TestValidator_StructValidation(t *testing.T) {
	// Valid struct
	err := validate.Struct(testStruct{Name: "test"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Invalid struct
	err = validate.Struct(testStruct{Name: ""})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}