package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"
	"github.com/google/uuid"

	"gostore/internal/config"
	"gostore/internal/core/domain"
	"gostore/internal/repository/sqlite"
)

func main() {
	flagUsername := flag.String("username", "", "Admin username")
	flagEmail := flag.String("email", "", "Admin email address")
	flagPassword := flag.String("password", "", "Admin password")
	flag.Parse()

	cfg := config.LoadConfig()

	db, err := sqlite.NewDatabase(cfg)
	if err != nil {
		fmt.Printf("❌ Error connecting to database: %v\n", err)
		os.Exit(1)
	}

	username := strings.TrimSpace(*flagUsername)
	email := strings.TrimSpace(*flagEmail)
	password := strings.TrimSpace(*flagPassword)

	reader := bufio.NewReader(os.Stdin)

	if username == "" || password == "" {
		fmt.Println("=====================================================")
		fmt.Println("⚡ GoStore CLI — Create Superuser (Admin)")
		fmt.Println("=====================================================")

		// 1. Username
		if username == "" {
			fmt.Print("Username: ")
			username, _ = reader.ReadString('\n')
			username = strings.TrimSpace(username)
			if username == "" {
				fmt.Println("❌ Error: Username cannot be blank.")
				os.Exit(1)
			}
		}

		// 2. Email
		if email == "" {
			fmt.Print("Email address: ")
			email, _ = reader.ReadString('\n')
			email = strings.TrimSpace(email)
		}

		// 3. Password
		if password == "" {
			fmt.Print("Password: ")
			bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil || len(bytePassword) == 0 {
				pStr, _ := reader.ReadString('\n')
				bytePassword = []byte(strings.TrimSpace(pStr))
			}
			password = strings.TrimSpace(string(bytePassword))

			if len(password) < 6 {
				fmt.Println("❌ Error: Password must be at least 6 characters long.")
				os.Exit(1)
			}

			// 4. Password confirmation
			fmt.Print("Password (again): ")
			bytePassword2, err := terminal.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil || len(bytePassword2) == 0 {
				pStr, _ := reader.ReadString('\n')
				bytePassword2 = []byte(strings.TrimSpace(pStr))
			}
			password2 := strings.TrimSpace(string(bytePassword2))

			if password != password2 {
				fmt.Println("❌ Error: Passwords do not match.")
				os.Exit(1)
			}
		}
	}

	if len(password) < 6 {
		fmt.Println("❌ Error: Password must be at least 6 characters long.")
		os.Exit(1)
	}

	// Generate bcrypt hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("❌ Error hashing password: %v\n", err)
		os.Exit(1)
	}

	userRepo := sqlite.NewUserRepository(db)
	ctx := context.Background()

	// Check if user already exists
	existing, err := userRepo.GetByUsername(ctx, username)
	if err == nil && existing != nil {
		existing.PasswordHash = string(hash)
		if email != "" {
			existing.Email = email
		}
		existing.IsSuperuser = true
		if err := userRepo.Update(ctx, existing); err != nil {
			fmt.Printf("❌ Error updating superuser: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Existing user updated to Superuser successfully.")
		fmt.Printf("👤 Username: %s\n", username)
		if existing.Email != "" {
			fmt.Printf("📧 Email:    %s\n", existing.Email)
		}
		fmt.Println("=====================================================")
		return
	}

	user := &domain.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		IsSuperuser:  true,
	}

	if err := userRepo.Create(ctx, user); err != nil {
		fmt.Printf("❌ Error creating superuser: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Superuser created successfully.")
	fmt.Printf("👤 Username: %s\n", username)
	fmt.Printf("📧 Email:    %s\n", email)
	fmt.Println("=====================================================")
}
