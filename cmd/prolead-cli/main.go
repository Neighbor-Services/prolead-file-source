package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	proleadfile "github.com/proleadfile/prolead-file/packages/go_prolead_file"
)

type Config struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".prolead.json"
	}
	dir := filepath.Join(home, ".prolead")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "config.json")
}

func loadConfig() Config {
	var cfg Config
	data, err := os.ReadFile(getConfigPath())
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	if cfg.APIKey == "" {
		cfg.APIKey = "prolead-master-secret-key"
	}
	return cfg
}

func saveConfig(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getConfigPath(), data, 0600)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cfg := loadConfig()
	client := proleadfile.NewClient(cfg.BaseURL, cfg.APIKey)
	ctx := context.Background()

	command := os.Args[1]

	switch command {
	case "login":
		loginCmd := flag.NewFlagSet("login", flag.ExitOnError)
		urlFlag := loginCmd.String("url", cfg.BaseURL, "Prolead File Server URL")
		keyFlag := loginCmd.String("key", cfg.APIKey, "Master or Project API Key")
		_ = loginCmd.Parse(os.Args[2:])

		newCfg := Config{
			BaseURL: strings.TrimRight(*urlFlag, "/"),
			APIKey:  *keyFlag,
		}
		if err := saveConfig(newCfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Logged in to Prolead File appliance at %s\n", newCfg.BaseURL)

	case "ls":
		bucket := "default"
		prefix := ""
		if len(os.Args) > 2 {
			arg := os.Args[2]
			parts := strings.SplitN(arg, "/", 2)
			bucket = parts[0]
			if len(parts) > 1 {
				prefix = parts[1]
			}
		}

		res, err := client.ListFiles(ctx, proleadfile.ListFilesFilter{
			Bucket: bucket,
			Prefix: prefix,
			Limit:  100,
		})
		if err != nil {
			fmt.Printf("Error listing files: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("--- Bucket: %s (Prefix: '%s') ---\n", bucket, prefix)
		for _, p := range res.Prefixes {
			fmt.Printf("[DIR]  %s\n", p)
		}
		for _, f := range res.Items {
			if strings.HasSuffix(f.Name, ".keep") {
				continue
			}
			fmt.Printf("[FILE] %-30s %10s  %s\n", f.Path, formatBytes(f.Size), f.UpdatedAt.Format(time.RFC822))
		}

	case "cp", "upload":
		if len(os.Args) < 4 {
			fmt.Println("Usage: prolead-cli cp <local-path> <bucket/remote-path>")
			os.Exit(1)
		}
		localPath := os.Args[2]
		remoteArg := os.Args[3]

		parts := strings.SplitN(remoteArg, "/", 2)
		bucket := parts[0]
		remotePath := filepath.Base(localPath)
		if len(parts) > 1 && parts[1] != "" {
			remotePath = parts[1]
		}

		fmt.Printf("Uploading %s -> %s/%s...\n", localPath, bucket, remotePath)
		obj, err := client.UploadLocalFile(ctx, bucket, remotePath, localPath)
		if err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Successfully uploaded! URL: %s\n", obj.DownloadURL)

	case "download":
		if len(os.Args) < 3 {
			fmt.Println("Usage: prolead-cli download <bucket/remote-path> [local-dest]")
			os.Exit(1)
		}
		remoteArg := os.Args[2]
		parts := strings.SplitN(remoteArg, "/", 2)
		if len(parts) < 2 {
			fmt.Println("Must specify bucket/object-path")
			os.Exit(1)
		}
		bucket := parts[0]
		remotePath := parts[1]

		dest := filepath.Base(remotePath)
		if len(os.Args) > 3 {
			dest = os.Args[3]
		}

		fmt.Printf("Downloading %s/%s -> %s...\n", bucket, remotePath, dest)
		bytes, err := client.DownloadBytes(ctx, bucket, remotePath)
		if err != nil {
			fmt.Printf("Download failed: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, bytes, 0644); err != nil {
			fmt.Printf("Save failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Download complete (%s)\n", formatBytes(int64(len(bytes))))

	case "sync":
		if len(os.Args) < 4 {
			fmt.Println("Usage: prolead-cli sync <local-directory> <bucket/prefix>")
			os.Exit(1)
		}
		localDir := os.Args[2]
		remoteArg := os.Args[3]
		parts := strings.SplitN(remoteArg, "/", 2)
		bucket := parts[0]
		prefix := ""
		if len(parts) > 1 {
			prefix = strings.TrimSuffix(parts[1], "/") + "/"
		}

		fmt.Printf("Syncing local directory '%s' to '%s/%s'...\n", localDir, bucket, prefix)
		var count int
		err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(localDir, path)
			destPath := prefix + filepath.ToSlash(rel)

			fmt.Printf("  -> Uploading %s...\n", destPath)
			_, err = client.UploadLocalFile(ctx, bucket, destPath, path)
			if err != nil {
				fmt.Printf("    Warning: failed to upload %s: %v\n", path, err)
			} else {
				count++
			}
			return nil
		})
		if err != nil {
			fmt.Printf("Sync error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Sync completed! %d files transferred.\n", count)

	case "rm", "delete":
		if len(os.Args) < 3 {
			fmt.Println("Usage: prolead-cli rm <bucket/remote-path>")
			os.Exit(1)
		}
		parts := strings.SplitN(os.Args[2], "/", 2)
		if len(parts) < 2 {
			fmt.Println("Must specify bucket/object-path")
			os.Exit(1)
		}
		if err := client.DeleteFile(ctx, parts[0], parts[1], false); err != nil {
			fmt.Printf("Delete failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Moved %s/%s to trash\n", parts[0], parts[1])

	case "share":
		if len(os.Args) < 3 {
			fmt.Println("Usage: prolead-cli share <bucket/remote-path> [durationSeconds]")
			os.Exit(1)
		}
		parts := strings.SplitN(os.Args[2], "/", 2)
		if len(parts) < 2 {
			fmt.Println("Must specify bucket/object-path")
			os.Exit(1)
		}
		dur := 3600
		res, err := client.GenerateSignedURL(ctx, parts[0], parts[1], dur)
		if err != nil {
			fmt.Printf("Share failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Generated Expiring Link (expires: %s):\n%s\n", res.ExpiresAt.Format(time.RFC822), res.SignedURL)

	case "stats":
		stats, err := client.GetStats(ctx)
		if err != nil {
			fmt.Printf("Failed to get stats: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("====================================================")
		fmt.Println(" Prolead File Appliance Telemetry & Deduplication")
		fmt.Println("====================================================")
		fmt.Printf(" Active Files:        %d\n", stats.TotalFiles)
		fmt.Printf(" Logical Data Volume: %s\n", formatBytes(stats.TotalLogicalBytes))
		fmt.Printf(" Physical CAS Data:   %s\n", formatBytes(stats.TotalPhysicalBytes))
		fmt.Printf(" Storage Space Saved: %s (Ratio: %.2fx)\n", formatBytes(stats.TotalSavedBytes), stats.DedupRatio)
		fmt.Printf(" Active Partitions:   %d buckets\n", stats.ActiveBuckets)
		fmt.Printf(" Revisions & Versions:%d snapshots\n", stats.ActiveVersions)
		fmt.Println("====================================================")

	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("Prolead File Command Line Utility (prolead-cli)")
	fmt.Println("Usage:")
	fmt.Println("  prolead-cli login --url <url> --key <key>       Set default appliance credentials")
	fmt.Println("  prolead-cli ls [bucket[/prefix]]               List objects and directories")
	fmt.Println("  prolead-cli cp <local-path> <bucket/remote>    Upload a file")
	fmt.Println("  prolead-cli download <bucket/remote> [local]   Download a file")
	fmt.Println("  prolead-cli sync <local-dir> <bucket/prefix>   Sync directory to bucket")
	fmt.Println("  prolead-cli rm <bucket/path>                   Delete an object")
	fmt.Println("  prolead-cli share <bucket/path>                Generate signed download URL")
	fmt.Println("  prolead-cli stats                              Display telemetry & dedup ratio")
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
