package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWsConnectHandlerRejectsInvalidRequestsBeforeUpgrade(t *testing.T) {
	server := NewSignalServer(slog.Default(), nil, nil)

	post := httptest.NewRequest(http.MethodPost, "/ws", nil)
	postRecorder := httptest.NewRecorder()
	server.WsConnectHandler(context.Background(), postRecorder, post, nil)
	if postRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", postRecorder.Code, http.StatusMethodNotAllowed)
	}

	missingName := httptest.NewRequest(http.MethodGet, "/ws", nil)
	missingNameRecorder := httptest.NewRecorder()
	server.WsConnectHandler(context.Background(), missingNameRecorder, missingName, nil)
	if missingNameRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing name status = %d, want %d", missingNameRecorder.Code, http.StatusBadRequest)
	}
}
