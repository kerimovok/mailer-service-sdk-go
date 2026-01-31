# Mailer Service SDK

A Go SDK for interacting with the mailer-service microservice. This SDK provides
a type-safe client for reading mails, templates, and attachments (list and
get-by-id).

## Installation

```bash
go get github.com/kerimovok/mailer-service-sdk-go
```

## Features

- **Type-safe**: Full type definitions for list/get responses (mails, templates,
  attachments)
- **Error handling**: `APIError` and `IsAPIError()` for API-level errors
- **Pagination**: List endpoints support page/per_page and optional filters via
  query string

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    mailersdk "github.com/kerimovok/mailer-service-sdk-go"
)

func main() {
    client, err := mailersdk.NewClient(mailersdk.Config{
        BaseURL: "http://localhost:3002",
        Timeout: 10 * time.Second,
    })
    if err != nil {
        panic(err)
    }

    ctx := context.Background()

    // List mails (query string: page, per_page, filters, etc.)
    resp, err := client.ListMails(ctx, "page=1&per_page=20")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Mails: %d items\n", len(resp.Data))

    // Get a mail by ID
    mail, err := client.GetMail(ctx, "mail-uuid")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Mail: %s\n", mail.Data.Subject)
}
```

## API Reference

### Mails

- **ListMails(ctx, queryString)** – Paginated list; pass query string (e.g.
  `page=1&per_page=20`, optional filters)
- **GetMail(ctx, id)** – Get a mail by ID

### Templates

- **ListTemplates(ctx, queryString)** – Paginated list; pass query string (e.g.
  `page=1&per_page=20`)
- **GetTemplate(ctx, id)** – Get a template by ID

### Attachments

- **ListAttachments(ctx, queryString)** – Paginated list; pass query string
  (e.g. `page=1&per_page=20&mail_id=...`)
- **GetAttachment(ctx, id)** – Get an attachment by ID

## Configuration

The SDK uses plain HTTP. Required:

- **BaseURL**: Mailer service base URL (e.g. `http://localhost:3002`)
- **Timeout**: Request timeout (optional, default 10s)

## Error Handling

```go
resp, err := client.GetMail(ctx, id)
if err != nil {
    if apiErr, ok := mailersdk.IsAPIError(err); ok {
        fmt.Printf("API Error (status %d): %s\n", apiErr.StatusCode, apiErr.Message)
    }
    return err
}
```

## License

MIT
