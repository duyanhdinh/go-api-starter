package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeoutSetsDeadlineAndCancelsAfterHandler(t *testing.T) {
	var received context.Context
	before := time.Now()
	handler := Timeout(time.Second)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received = request.Context()
		deadline, exists := received.Deadline()
		if !exists || deadline.Before(before.Add(time.Second)) || deadline.After(time.Now().Add(time.Second)) {
			t.Fatalf("unexpected deadline: %v, exists=%v", deadline, exists)
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusAccepted || received.Err() != context.Canceled {
		t.Fatalf("status=%d, context error=%v", recorder.Code, received.Err())
	}
}

func TestTimeoutPreservesParentCancellationAndEarlierDeadline(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	deadline, _ := parent.Deadline()
	handler := Timeout(time.Hour)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		actual, _ := request.Context().Deadline()
		if !actual.Equal(deadline) || request.Context().Err() != context.DeadlineExceeded {
			t.Fatalf("deadline=%v, error=%v", actual, request.Context().Err())
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(parent))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("timeout replaced handler response: %d", recorder.Code)
	}
}
