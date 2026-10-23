package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	emailstore "github.com/dujiao-next/internal/modules/identity/emailverification/infrastructure/gormstore"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	notificationsmtp "github.com/dujiao-next/internal/modules/notification/infrastructure/smtp"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsstore "github.com/dujiao-next/internal/modules/settings/infrastructure/gormstore"
	settingsmessaging "github.com/dujiao-next/internal/modules/settings/schema/messaging"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type identityBackupArtifact struct {
	Path            string `json:"path"`
	SHA256          string `json:"sha256"`
	RestoreVerified bool   `json:"restore_verified"`
}

type identityBackupManifest struct {
	Batch              string                 `json:"batch"`
	CreatedAt          time.Time              `json:"created_at"`
	IdentityLinksEmpty bool                   `json:"identity_links_empty"`
	Postgres           identityBackupArtifact `json:"postgres"`
	Shop               identityBackupArtifact `json:"shop"`
}

// Both backups must have been restored in isolation before the explicit first
// identity migration. This command never migrates business tables or users.
func verifyIdentityBackup(path string, now time.Time) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("paired backup manifest unavailable")
	}
	defer file.Close()
	var manifest identityBackupManifest
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF || manifest.Batch == "" || !manifest.IdentityLinksEmpty || manifest.CreatedAt.After(now) || now.Sub(manifest.CreatedAt) > time.Hour {
		return errors.New("paired backup manifest invalid or older than one hour")
	}
	for _, artifact := range []identityBackupArtifact{manifest.Postgres, manifest.Shop} {
		if !artifact.RestoreVerified || !filepath.IsAbs(artifact.Path) || len(artifact.SHA256) != 64 {
			return errors.New("isolated restore evidence missing")
		}
		backup, err := os.Open(artifact.Path)
		if err != nil {
			return errors.New("paired backup unavailable")
		}
		info, statErr := backup.Stat()
		hash := sha256.New()
		_, copyErr := io.Copy(hash, backup)
		closeErr := backup.Close()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 || copyErr != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
			return errors.New("paired backup digest mismatch")
		}
	}
	if manifest.Postgres.Path == manifest.Shop.Path {
		return errors.New("distinct database backups required")
	}
	return nil
}

func runIdentityCommand(args []string) int {
	if len(args) == 2 && args[0] == "send-registration-test" && args[1] == "--send" {
		return sendIdentityRegistrationTest()
	}
	flags := flag.NewFlagSet("identity migrate", flag.ContinueOnError)
	apply := flags.Bool("apply", false, "create identity schema after paired recovery verification")
	manifest := flags.String("backup-manifest", "", "fresh paired backup and isolated restore manifest")
	if len(args) == 0 || args[0] != "migrate" || flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !*apply || *manifest == "" {
		fmt.Fprintln(os.Stderr, "usage: identity migrate --apply --backup-manifest PATH")
		return 2
	}
	if err := verifyIdentityBackup(*manifest, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	cfg := config.Load()
	db, err := openIdentityDatabase(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity database unavailable")
		return 1
	}
	pool, err := db.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity database unavailable")
		return 1
	}
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if pool.PingContext(ctx) != nil || federationstore.New(db).Migrate(ctx) != nil {
		fmt.Fprintln(os.Stderr, "identity migration failed; preserve backups and inspect schema before retry")
		return 1
	}
	fmt.Println(`{"identity_schema":"ready","business_accounts_unchanged":true}`)
	return 0
}

func openIdentityDatabase(cfg *config.Config) (*gorm.DB, error) {
	var driver gorm.Dialector
	switch cfg.Database.Driver {
	case "sqlite":
		driver = sqlite.Open(cfg.Database.DSN)
	case "postgres", "postgresql":
		driver = postgres.Open(cfg.Database.DSN)
	default:
		return nil, errors.New("unsupported identity database driver")
	}
	return gorm.Open(driver, &gorm.Config{Logger: logger.Discard, DisableAutomaticPing: true})
}

// The recipient is read only from stdin, never printed, logged or placed in
// process arguments. This sends a real registration proof but creates no user.
func sendIdentityRegistrationTest() int {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 256))
	if err != nil || len(input) > 254 {
		fmt.Fprintln(os.Stderr, "one test recipient required on stdin")
		return 2
	}
	email := strings.TrimSpace(string(input))
	if !strings.Contains(email, "@") || strings.ContainsAny(email, "\r\n") {
		fmt.Fprintln(os.Stderr, "one test recipient required on stdin")
		return 2
	}
	cfg := config.Load()
	db, err := openIdentityDatabase(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity database unavailable")
		return 1
	}
	pool, err := db.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity database unavailable")
		return 1
	}
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	settings := settingsapp.NewService(settingsstore.New(db), cfg.Order)
	smtp, err := settings.GetSMTPSetting(cfg.Email)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mail configuration unavailable")
		return 1
	}
	cfg.Email = settingsmessaging.SMTPSettingToConfig(smtp)
	service := userauthapp.NewService(cfg, userstore.New(db), nil, emailstore.New(db), settings, notificationsmtp.New(&cfg.Email), nil)
	err = service.SendRegistrationCodeForSite(ctx, email, "zh-CN", mailbrand.Brand{SiteName: "绘画荟", SiteURL: "https://www.huihuahui.xyz", FromName: "绘画荟"})
	// Existing recipients use the established mailbox re-verification path.
	// Sending its code does not change their password or verification status.
	if errors.Is(err, userauthapp.ErrEmailExists) {
		err = service.SendVerifyCode(ctx, email, constants.VerifyPurposeReset, "zh-CN")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "registration test mail was not sent; inspect sanitized mail diagnostics")
		return 1
	}
	fmt.Println(`{"registration_test_mail":"accepted","account_created":false}`)
	return 0
}
