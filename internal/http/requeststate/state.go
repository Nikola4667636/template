package requeststate

import (
	"context"
	"fmt"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type contextKey struct{}

// State belongs to one request. Completion hooks run after request recovery.
type State struct {
	Request     *http.Request
	Writer      chimiddleware.WrapResponseWriter
	Route       string
	Err         error
	Operation   string
	Panicked    bool
	LogsManaged bool
	completion  []func()
}

func Begin(w http.ResponseWriter, r *http.Request) *State {
	s := &State{Writer: chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor), Route: "/unmatched"}
	s.Request = r.WithContext(context.WithValue(r.Context(), contextKey{}, s))
	return s
}

func FromContext(ctx context.Context) *State {
	s, _ := ctx.Value(contextKey{}).(*State)
	return s
}

func (s *State) OnComplete(fn func()) {
	s.completion = append(s.completion, fn)
}

func (s *State) Complete() {
	for i := len(s.completion) - 1; i >= 0; i-- {
		s.completion[i]()
	}
}

func (s *State) Status() int {
	if status := s.Writer.Status(); status != 0 {
		return status
	}
	return http.StatusOK
}

func RecordError(ctx context.Context, op string, err error) bool {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, "request failed")
	span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
	if state := FromContext(ctx); state != nil {
		state.Err, state.Operation = err, op
		return true
	}
	return false
}
