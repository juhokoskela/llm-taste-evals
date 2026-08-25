# Frozen merge contract

- Preserve `FilesService.Add` with its existing signature and behavior. It
  remains the lower-level API for callers that build custom multipart bodies.
- Add exactly this convenience API:
  `Upload(ctx context.Context, fileName string, content io.Reader, opts ...FilesOption) (*File, error)`.
- Encode `content` as multipart form data in the field named `file`, using
  `fileName` as the uploaded filename.
- The request body produced by `Upload` must be replayable so the existing
  retry transport can resend it after a 429 when retries are enabled.
- Buffering the uploaded content in memory is acceptable; typical files are a
  few megabytes, and existing recording and image upload paths do the same.
- Reuse `internal/multipartbody.NewFile`, the established helper already used
  by call-log recording and product image uploads.
- Route the encoded body through the existing `FilesService.Add` method so its
  option handling, request construction, response decoding, and error behavior
  stay in one place.
- Validate an empty filename and nil content before making a request. Propagate
  source-reader failures.
- Keep `FilesOption` behavior unchanged. Do not add association fields, change
  `Update`, deprecate `Add`, or refactor unrelated request/retry machinery.
- Tests should cover multipart field/name/content, streamed-content retry,
  validation, and source-reader errors without weakening existing tests.
