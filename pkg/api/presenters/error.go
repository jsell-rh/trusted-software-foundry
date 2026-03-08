package presenters

import (
	"github.com/jsell-rh/trusted-software-components/pkg/api/openapi"
	"github.com/jsell-rh/trusted-software-components/pkg/errors"
)

func PresentError(err *errors.ServiceError) openapi.Error {
	return err.AsOpenapiError("")
}
