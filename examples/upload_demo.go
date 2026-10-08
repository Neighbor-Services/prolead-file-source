package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"gostore/pkg/client"
)

func main() {
	serverURL := "http://localhost:8080"
	apiKey := "gostore-master-secret-key"

	c := client.NewClient(serverURL, apiKey)

	fmt.Println("1. Uploading test file to GoStore...")
	sampleContent := strings.NewReader("Hello from GoStore! This file was uploaded directly using the Go Client SDK.")

	fileObj, err := c.UploadFile(
		context.Background(),
		"default",
		"documents/welcome.txt",
		sampleContent,
		"text/plain",
	)
	if err != nil {
		log.Fatalf("Upload failed: %v", err)
	}

	fmt.Printf("✅ Uploaded successfully!\n")
	fmt.Printf("   File ID:       %s\n", fileObj.ID)
	fmt.Printf("   Size:          %d bytes\n", fileObj.Size)
	fmt.Printf("   Content-Type:  %s\n", fileObj.ContentType)
	fmt.Printf("   Download Token:%s\n", fileObj.DownloadToken)
	fmt.Printf("   Download URL:  %s\n", fileObj.DownloadURL)

	fmt.Println("\n2. Listing files in 'default' bucket...")
	listRes, err := c.ListFiles(context.Background(), "default", "")
	if err != nil {
		log.Fatalf("Listing failed: %v", err)
	}
	fmt.Printf("Found %d file(s):\n", len(listRes.Items))
	for _, item := range listRes.Items {
		fmt.Printf(" - %s (%d bytes) -> %s\n", item.Path, item.Size, item.DownloadURL)
	}
}
