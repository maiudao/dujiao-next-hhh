package emailverificationstore

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"

	emailverificationcontract "github.com/dujiao-next/internal/modules/identity/emailverification/contract"
	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"

	"gorm.io/gorm"
)

// WithSendLock serializes the first send as well as resends across processes.
// The SMTP call is not retried by this transaction: retrying it could send the
// same request twice after a database/connection failure.
func (store *Store) WithSendLock(ctx context.Context, email, purpose string, send func(emailverificationcontract.Store) error) error {
	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		switch tx.Dialector.Name() {
		case "postgres":
			digest := sha256.Sum256([]byte("email-code\x00" + email + "\x00" + purpose))
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(digest[:8]))).Error; err != nil {
				return err
			}
		case "sqlite":
			if err := tx.Exec("UPDATE email_verify_codes SET id=id WHERE 0=1").Error; err != nil {
				return err
			}
		default:
			return errors.New("verification send locking is unavailable")
		}
		return send(New(tx))
	})
}

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (store *Store) Create(code *emailverificationdomain.Code) error {
	return store.db.Create(code).Error
}

func (store *Store) GetLatest(email, purpose string) (*emailverificationdomain.Code, error) {
	var record emailverificationdomain.Code
	if err := store.db.Where("email = ? AND purpose = ? AND deleted_at IS NULL", email, purpose).
		Order("sent_at desc, id desc").
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (store *Store) MarkVerified(id uint, verifiedAt time.Time) error {
	return store.db.Model(&emailverificationdomain.Code{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("verified_at", verifiedAt).Error
}

func (store *Store) IncrementAttempt(id uint) error {
	return store.db.Model(&emailverificationdomain.Code{}).
		Where("id = ? AND deleted_at IS NULL", id).
		UpdateColumn("attempt_count", gorm.Expr("attempt_count + 1")).Error
}
