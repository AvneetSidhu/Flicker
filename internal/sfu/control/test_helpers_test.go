package control

import "log/slog"

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(testDiscardWriter{}, nil))
}

type testDiscardWriter struct{}

func (testDiscardWriter) Write(data []byte) (int, error) {
	return len(data), nil
}
