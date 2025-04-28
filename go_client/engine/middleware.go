package engine

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go_client/pkg/result"
	"net/http"
	"runtime/debug"
	"time"
)

func RecoveryMiddleware(logger *zap.Logger) gin.RecoveryFunc {
	return func(c *gin.Context, err any) {
		response := result.New[any](http.StatusServiceUnavailable)
		logger.Error(fmt.Sprintf("Recovery panic:%s", string(debug.Stack())))
		response.Message("Service Unavailable").Err(c.Writer)
		c.Abort()
	}
}

func LoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now().Local()
		c.Next()
		now := time.Now().Local()

		logger.Debug(fmt.Sprintf("|%d| %s\t%s: %s %s",
			c.Writer.Status(),
			c.ClientIP(),
			c.Request.Method,
			c.Request.RequestURI,
			now.Sub(start).String()),
		)

	}
}
