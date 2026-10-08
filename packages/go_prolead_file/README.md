# go_prolead_file

Official Go client library for **Prolead File** on-premise high-performance object storage appliance.

---

## 🚀 Features

- **Bucket Management**: Full lifecycle management of buckets, quotas, and MIME filters.
- **Multipart Streaming Upload**: Stream uploads directly from any `io.Reader` with custom metadata.
- **Dynamic Image Transformations**: Format transcoding, intelligent cropping, resizing, and quality tuning.
- **HMAC Signed URLs**: Generate expiring pre-authenticated URLs.
- **Server-Sent Events (SSE)**: Channel-based real-time event listener.

---

## 📦 Installation

```bash
go get github.com/proleadfile/prolead-file/packages/go_prolead_file
```

---

## 🛠️ Quick Start

```go
package main

import (
	"context"
	"fmt"
	"strings"

	proleadfile "github.com/proleadfile/prolead-file/packages/go_prolead_file"
)

func main() {
	client := proleadfile.NewClient("http://localhost:8080", "prolead-master-secret-key")

	// 1. Upload file
	reader := strings.NewReader("Hello from Prolead File Go SDK!")
	file, err := client.UploadFile(context.Background(), "default", "logs/app.txt", reader, "text/plain")
	if err != nil {
		panic(err)
	}
	fmt.Println("Uploaded:", file.DownloadURL)

	// 2. Stream real-time storage events
	events, errs, err := client.SubscribeEvents(context.Background())
	if err != nil {
		panic(err)
	}

	for event := range events {
		fmt.Printf("Event [%s]: %s in bucket %s\n", event.EventType, event.Path, event.Bucket)
	}
}
```
