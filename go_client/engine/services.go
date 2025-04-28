package engine

import (
	"github.com/gin-gonic/gin"
	"go_client/pkg/result"
	"go_client/pkg/status"
	"net/http"
)

type HandlerFunc func(c *gin.Context) error

func WrapHandler(fc HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := fc(c)
		if err == nil {
			return
		}

		if statusErr, ok := status.As(err); ok {
			result.Any(statusErr.StatusCode).Message(statusErr.Error()).Err(c.Writer)
			return
		}
		
		result.Any(http.StatusUnauthorized).Message(err.Error()).Err(c.Writer)
	}
}
