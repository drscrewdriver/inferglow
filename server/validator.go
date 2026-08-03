package server

import "github.com/go-playground/validator/v10"

// validate is the shared validator instance for request validation.
var validate = validator.New()