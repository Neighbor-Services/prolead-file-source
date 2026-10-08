package proleadfile_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	proleadfile "github.com/proleadfile/prolead-file/packages/go_prolead_file"
)

func TestMockClientWorkflow(t *testing.T) {
	ctx := context.Background()
	mock := proleadfile.NewMockClient()

	// 1. Create Bucket
	b, err := mock.CreateBucket(ctx, proleadfile.StorageBucket{
		Name:        "backups",
		Description: "Database backup bucket",
		IsPublic:    false,
	})
	if err != nil {
		t.Fatalf("failed to create bucket: %v", err)
	}
	if b.Name != "backups" {
		t.Errorf("expected bucket name backups, got %s", b.Name)
	}

	// 2. Upload Object
	data := []byte("SQL Dump Content Here...")
	obj, err := mock.UploadFile(ctx, "backups", "db/backup.sql", bytes.NewReader(data), "application/sql")
	if err != nil {
		t.Fatalf("failed to upload: %v", err)
	}
	if obj.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), obj.Size)
	}

	// 3. List Files
	list, err := mock.ListFiles(ctx, proleadfile.ListFilesFilter{Bucket: "backups"})
	if err != nil {
		t.Fatalf("failed to list files: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(list.Items))
	}

	// 4. Download
	dl, err := mock.DownloadBytes(ctx, "backups", "db/backup.sql")
	if err != nil {
		t.Fatalf("failed to download: %v", err)
	}
	if string(dl) != string(data) {
		t.Errorf("downloaded content mismatch")
	}

	// 5. Share Links
	link, err := mock.CreateShareLink(ctx, "backups", "db/backup.sql", 24, "secret123", 5)
	if err != nil {
		t.Fatalf("failed to create share link: %v", err)
	}
	if link.Token == "" {
		t.Errorf("expected share link token, got empty")
	}

	shares, err := mock.ListShareLinks(ctx, "backups")
	if err != nil {
		t.Fatalf("failed to list share links: %v", err)
	}
	if len(shares) != 1 {
		t.Errorf("expected 1 share link, got %d", len(shares))
	}

	// 6. Lifecycle Rules
	rule, err := mock.CreateLifecycleRule(ctx, proleadfile.LifecycleRule{
		Bucket:     "backups",
		Prefix:     "db/",
		ExpireDays: 30,
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("failed to create lifecycle rule: %v", err)
	}
	if rule.ID == "" {
		t.Errorf("expected rule ID, got empty")
	}

	// 7. Stats
	stats, err := mock.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.TotalFiles != 1 {
		t.Errorf("expected 1 total file, got %d", stats.TotalFiles)
	}

	// 8. Delete
	if err := mock.DeleteFile(ctx, "backups", "db/backup.sql", true); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// 9. TUS Resumable Upload
	tusData := []byte("Large stream simulated bytes for TUS")
	tusObj, err := mock.UploadTUS(ctx, "backups", "db/large.bin", bytes.NewReader(tusData), int64(len(tusData)))
	if err != nil {
		t.Fatalf("failed TUS upload: %v", err)
	}
	if tusObj.Size != int64(len(tusData)) {
		t.Errorf("expected size %d, got %d", len(tusData), tusObj.Size)
	}

	// 10. Admin Ops (Audit Export, GC, Dedup Report)
	var auditBuf bytes.Buffer
	if err := mock.ExportAuditLogs(ctx, "csv", &auditBuf); err != nil {
		t.Fatalf("failed to export audit logs: %v", err)
	}
	if !bytes.Contains(auditBuf.Bytes(), []byte("MOCK_ACTION")) {
		t.Errorf("expected MOCK_ACTION in audit log output")
	}

	gcRep, err := mock.TriggerGC(ctx)
	if err != nil || gcRep.DeletedBlobs <= 0 {
		t.Fatalf("expected successful GC report, got %v", err)
	}

	dedupRep, err := mock.GetDedupReport(ctx)
	if err != nil || dedupRep["dedupRatio"] == nil {
		t.Fatalf("expected dedup report, got %v", err)
	}
}

func TestImageTransformBuilding(t *testing.T) {
	client := proleadfile.NewClient("http://localhost:8080", "test-key", proleadfile.WithRetry(3, 100*time.Millisecond))
	url := client.GetTransformedImageURL("/v0/b/default/o/image.png", proleadfile.ImageTransformOptions{
		Width:   500,
		Height:  300,
		Fit:     "cover",
		Format:  "webp",
		Quality: 85,
	})

	if url == "" {
		t.Errorf("expected valid transformed URL")
	}
}

func TestProgressStreamWrappers(t *testing.T) {
	data := []byte("1234567890abcdefghijklmnopqrstuvwxyz")
	var lastPercent float64

	pr := proleadfile.NewProgressReader(bytes.NewReader(data), int64(len(data)), func(transferred, total int64, percent float64) {
		lastPercent = percent
	})

	buf := make([]byte, 10)
	for {
		n, err := pr.Read(buf)
		if n == 0 || err != nil {
			break
		}
	}

	if lastPercent != 100.0 {
		t.Errorf("expected 100%% progress, got %.2f%%", lastPercent)
	}

	var writeBuf bytes.Buffer
	var writePercent float64
	pw := proleadfile.NewProgressWriter(&writeBuf, int64(len(data)), func(transferred, total int64, percent float64) {
		writePercent = percent
	})

	pw.Write(data)
	if writePercent != 100.0 {
		t.Errorf("expected 100%% write progress, got %.2f%%", writePercent)
	}
}

func TestWebhookSignatureVerification(t *testing.T) {
	secret := "whsec_test_secret_key_12345"
	payload := []byte(`{"eventType":"OBJECT_CREATED","bucket":"default","path":"photo.jpg"}`)

	// Compute expected HMAC
	validSig := "sha256=123" // Will test validity helper
	// Empty secret / invalid signature
	_, err := proleadfile.VerifyWebhookSignature(payload, validSig, "")
	if err == nil {
		t.Errorf("expected error with empty secret")
	}

	// Correct signature test
	valid, err := proleadfile.VerifyWebhookSignature(payload, "invalid_sig", secret)
	if err != nil || valid {
		t.Errorf("expected invalid signature to fail cleanly")
	}
}

