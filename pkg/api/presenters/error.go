package presenters

import (
	"github.com/jsell-rh/trusted-software-foundry/pkg/api/openapi"
	"github.com/jsell-rh/trusted-software-foundry/pkg/errors"
)

func PresentError(err *errors.ServiceError) openapi.Error {
	return err.AsOpenapiError("")
}
