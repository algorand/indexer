package middlewares

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
)

type loggerMiddleware struct {
	log *log.Logger
}

// MakeLogger initializes a logger echo.MiddlewareFunc
func MakeLogger(log *log.Logger) echo.MiddlewareFunc {
	logger := loggerMiddleware{
		log: log,
	}

	return logger.handler
}

// Logger is an echo middleware to add log to the API
func (logger *loggerMiddleware) handler(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) (err error) {
		start := time.Now()

		// Get a reference to the response object.
		res := ctx.Response()
		req := ctx.Request()

		// Propogate the error if the next middleware has a problem
		if err = next(ctx); err != nil {
			ctx.Error(err)
		}

		logger.log.Infof("%s %s %s [%v] \"%s %s %s\" %d %s \"%s\" %s",
			req.RemoteAddr,
			"-",
			"-",
			start,
			req.Method,
			req.RequestURI,
			req.Proto, // string "HTTP/1.1"
			res.Status,
			strconv.FormatInt(res.Size, 10), // bytes_out
			req.UserAgent(),
			time.Since(start),
		)

		return
	}
}
