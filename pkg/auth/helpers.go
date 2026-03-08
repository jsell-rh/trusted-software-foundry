package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jsell-rh/trusted-software-components/pkg/errors"
	"github.com/jsell-rh/trusted-software-components/pkg/logger"
)

func handleError(ctx context.Context, w http.ResponseWriter, code errors.ServiceErrorCode, reason string) {
	log := logger.NewLogger(ctx)
	operationID := logger.GetOperationID(ctx)
	err := errors.New(code, "%s", reason)
	if err.HttpCode >= 400 && err.HttpCode <= 499 {
		log.Infof(err.Error())
	} else {
		log.Error(err.Error())
	}

	writeJSONResponse(w, err.HttpCode, err.AsOpenapiError(operationID))
}

func writeJSONResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if payload != nil {
		response, _ := json.Marshal(payload)
		_, _ = w.Write(response)
	}
}
