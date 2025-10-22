package middlewares

import (
	"bufio"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/balazskvancz/gorouter"
)

const (
	microNanoTreshold      int64         = 10_000
	defaultBufferFlushFreq time.Duration = 500 * time.Millisecond
)

// Logger creates and returns a middleware which logs
// information about the incoming request and the response.
func Logger(w ...io.Writer) gorouter.Middleware {
	var writer io.Writer = os.Stdout
	if len(w) > 0 {
		writer = w[0]
	}

	l := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{}))

	return gorouter.NewMiddleware(
		func(ctx gorouter.Context) {
			i := ctx.GetInfo()

			var (
				elapsedTime = time.Since(i.StartTime)

				timeValue       = elapsedTime.Microseconds()
				measurementUnit = "µs"
			)

			// By default, the unit of measuremnt is in microseconds however,
			// if the value exceeds the treshold value, which is 10 000µs
			// then it is promoted to milliseconds.
			//
			// Maybe later this treshold value will be configurable.
			if timeValue > microNanoTreshold {
				measurementUnit = "ms"
				timeValue = elapsedTime.Milliseconds()
			}

			l.Info("incoming request",
				"id", i.Id,
				"method", i.Method,
				"url", i.Url,
				"status", i.StatusCode,
				"response_bytes", i.WrittenBytes,
				measurementUnit, timeValue,
			)

			ctx.Next()
		},
		gorouter.MiddlewareWithType(gorouter.MiddlewarePostRunner),
	)
}

type BufferedLoggerOptions struct {
	FlushFrequency time.Duration
	Writer         io.Writer
}

// BufferedLogger creates and returns a middleware on top of
// the `Logger` middleware that provides buffered logging,
// where a separate goroutine is launched and flushes the
// the content of the buffered reader everytime the provided
// duration passed.
func BufferedLogger(opts *BufferedLoggerOptions) gorouter.Middleware {
	var (
		waitDuration           = defaultBufferFlushFreq
		w            io.Writer = os.Stdout
	)

	if opts != nil {
		if opts.FlushFrequency != 0 {
			waitDuration = opts.FlushFrequency
		}

		if opts.Writer != nil {
			w = opts.Writer
		}
	}

	buff := bufio.NewWriter(w)
	go func() {
		for range time.Tick(waitDuration) {
			buff.Flush()
		}
	}()

	return Logger(buff)
}
