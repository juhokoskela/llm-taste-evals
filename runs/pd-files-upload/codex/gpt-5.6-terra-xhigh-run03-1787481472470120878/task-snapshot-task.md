# File uploads fail to retry

You maintain `pipedrive-go`, a Go SDK for the Pipedrive CRM API.

A user of the SDK reports:

> We upload documents to Pipedrive with `client.Files.Add`, reading from files we
> open with `os.Open`. We enabled retries with `RetryPolicy{RetryAllMethods: true}`
> because Pipedrive rate-limits us during bulk imports, but our uploads still fail
> immediately on 429 instead of retrying — every other call we make retries fine.
>
> Separately, `Files.Add` is awkward to use: we have to build the multipart body
> ourselves. Call-log recording uploads just take a file name and a reader.

Address the report.
