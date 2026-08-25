package v1

// Hidden evaluation tests for the files-upload task. This file is overlaid
// onto the candidate's tree after the agent finishes; it must stay fully
// self-contained (own helpers, TestHidden_ prefix) so it can never collide
// with agent-written tests.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juhokoskela/pipedrive-go/pipedrive"
)

func hiddenTestClient(t *testing.T, cfg pipedrive.Config, handler http.HandlerFunc) *Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg.BaseURL = srv.URL
	cfg.HTTPClient = srv.Client()

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	return client
}

type hiddenFailingReader struct{ err error }

func (r *hiddenFailingReader) Read([]byte) (int, error) { return 0, r.err }

func TestHidden_FilesUpload_EncodesSingleFilePart(t *testing.T) {
	t.Parallel()

	client := hiddenTestClient(t, pipedrive.Config{}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/files" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("body is not multipart/form-data: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		part, err := mr.NextPart()
		if err != nil {
			t.Errorf("missing file part: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			t.Errorf("form field = %q, want %q", part.FormName(), "file")
		}
		if part.FileName() != "hidden-report.pdf" {
			t.Errorf("file name = %q, want %q", part.FileName(), "hidden-report.pdf")
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Errorf("read part: %v", err)
		}
		if string(data) != "hidden-payload" {
			t.Errorf("content = %q, want %q", data, "hidden-payload")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":99,"name":"hidden-report.pdf"}}`))
	})

	file, err := client.Files.Upload(context.Background(), "hidden-report.pdf", strings.NewReader("hidden-payload"))
	if err != nil {
		t.Fatalf("Upload error: %v", err)
	}
	if file == nil || file.ID != 99 || file.Name != "hidden-report.pdf" {
		t.Fatalf("unexpected file: %#v", file)
	}
}

func TestHidden_FilesUpload_RetriesNonSeekableContent(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var replayed atomic.Bool
	client := hiddenTestClient(t, pipedrive.Config{
		RetryPolicy: &pipedrive.RetryPolicy{
			MaxAttempts:     2,
			BaseDelay:       time.Millisecond,
			MaxDelay:        2 * time.Millisecond,
			RetryAllMethods: true,
		},
	}, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		mr, err := r.MultipartReader()
		if err == nil {
			if part, perr := mr.NextPart(); perr == nil {
				data, _ := io.ReadAll(part)
				replayed.Store(string(data) == "hidden-streamed-payload")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"name":"stream.bin"}}`))
	})

	// io.MultiReader hides the concrete reader type, so net/http cannot
	// derive GetBody on its own; replayability must come from the SDK.
	content := io.MultiReader(strings.NewReader("hidden-streamed-payload"))
	if _, err := client.Files.Upload(context.Background(), "stream.bin", content); err != nil {
		t.Fatalf("Upload error (upload was not retried?): %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2 (429 must be retried)", got)
	}
	if !replayed.Load() {
		t.Fatal("retried request did not carry the full multipart body")
	}
}

func TestHidden_FilesUpload_RejectsMissingInput(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client := hiddenTestClient(t, pipedrive.Config{}, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1,"name":"x"}}`))
	})

	if _, err := client.Files.Upload(context.Background(), "", strings.NewReader("x")); err == nil {
		t.Error("expected error for empty file name")
	}
	// Must return an error, not panic (the panic is recovered here only to
	// report it as a clean failure).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Upload panicked on nil content: %v", r)
			}
		}()
		if _, err := client.Files.Upload(context.Background(), "a.txt", nil); err == nil {
			t.Error("expected error for nil content")
		}
	}()
	if got := requests.Load(); got != 0 {
		t.Errorf("invalid input reached the server (%d requests)", got)
	}
}

func TestHidden_FilesUpload_PropagatesSourceReadError(t *testing.T) {
	t.Parallel()

	client := hiddenTestClient(t, pipedrive.Config{}, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":1,"name":"x"}}`))
	})

	readErr := errors.New("hidden-read-failure")
	_, err := client.Files.Upload(context.Background(), "a.txt", &hiddenFailingReader{err: readErr})
	if err == nil {
		t.Fatal("expected source read error")
	}
	if !errors.Is(err, readErr) && !strings.Contains(err.Error(), "hidden-read-failure") {
		t.Fatalf("source read error not surfaced, got: %v", err)
	}
}

func TestHidden_FilesAdd_LegacyRawBodyPreserved(t *testing.T) {
	t.Parallel()

	contentType := "multipart/form-data; boundary=hiddenlegacy"
	client := hiddenTestClient(t, pipedrive.Config{}, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != contentType {
			t.Errorf("content type = %q, want %q", got, contentType)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "raw-legacy-body" {
			t.Errorf("body = %q, want %q", body, "raw-legacy-body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":12,"name":"legacy.txt"}}`))
	})

	file, err := client.Files.Add(context.Background(), strings.NewReader("raw-legacy-body"), contentType)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if file.ID != 12 || file.Name != "legacy.txt" {
		t.Fatalf("unexpected file: %#v", file)
	}
}
