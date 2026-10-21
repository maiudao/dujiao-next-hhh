package federationstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrInteraction = federationdomain.ErrInteraction

const (
	StagePending       = federationdomain.StagePending
	StageSecondFactor  = federationdomain.StageSecondFactor
	StageAuthenticated = federationdomain.StageAuthenticated
	StageCodeIssued    = federationdomain.StageCodeIssued
	MaxAttempts        = 5
)

type Store struct {
	db  *gorm.DB
	now func() time.Time
}

func New(db *gorm.DB) *Store {
	// Protocol state and browser/code hashes must never appear in SQL traces.
	// Return errors to the transport for request-ID-based, sanitized diagnostics.
	return &Store{db: db.Session(&gorm.Session{Logger: logger.Discard}), now: time.Now}
}

func Secret() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func validSecret(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *Store) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&federationdomain.Subject{}, &federationdomain.BrowserSession{}, &federationdomain.Interaction{}, &federationdomain.AccessToken{})
}

// Begin only persists an already validated OIDC request. The OIDC adapter must
// enforce registered clients, redirects, scopes and S256 before calling it.
func (s *Store) Begin(ctx context.Context, clientID string, requestJSON []byte, browser, csrf string) (*federationdomain.Interaction, error) {
	if clientID == "" || !json.Valid(requestJSON) || len(requestJSON) > 16384 || !validSecret(browser) || !validSecret(csrf) {
		return nil, ErrInteraction
	}
	id, err := Secret()
	if err != nil {
		return nil, err
	}
	now := s.now()
	value := &federationdomain.Interaction{ID: id, ClientID: clientID, RequestJSON: string(requestJSON), BrowserHash: Hash(browser), CSRFHash: Hash(csrf), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute), Stage: StagePending}
	if err := s.db.WithContext(ctx).Create(value).Error; err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Store) ReadBound(ctx context.Context, id, clientID, browser, csrf string) (*federationdomain.Interaction, error) {
	if !validSecret(browser) || !validSecret(csrf) {
		return nil, ErrInteraction
	}
	var value federationdomain.Interaction
	if err := s.db.WithContext(ctx).Where("id = ? AND client_id = ? AND expires_at > ? AND cancelled_at IS NULL AND code_consumed_at IS NULL", id, clientID, s.now()).First(&value).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(value.BrowserHash), []byte(Hash(browser))) != 1 || subtle.ConstantTimeCompare([]byte(value.CSRFHash), []byte(Hash(csrf))) != 1 {
		return nil, ErrInteraction
	}
	if value.Attempts >= MaxAttempts && value.Stage != StageAuthenticated && value.Stage != StageCodeIssued {
		return nil, ErrInteraction
	}
	return &value, nil
}

func changed(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInteraction
	}
	return nil
}

// gate rechecks browser, CSRF, lifetime, cancellation and attempt budget inside
// every conditional write, so a stale pre-read cannot authorize a transition.
func (s *Store) gate(db *gorm.DB, value *federationdomain.Interaction) *gorm.DB {
	return s.boundGate(db, value).Where("(attempts < ? OR stage IN ?)", MaxAttempts, []string{StageAuthenticated, StageCodeIssued})
}

func (s *Store) boundGate(db *gorm.DB, value *federationdomain.Interaction) *gorm.DB {
	return db.Model(&federationdomain.Interaction{}).Where("id = ? AND client_id = ? AND browser_hash = ? AND csrf_hash = ? AND expires_at > ? AND cancelled_at IS NULL AND code_consumed_at IS NULL", value.ID, value.ClientID, value.BrowserHash, value.CSRFHash, s.now())
}

func (s *Store) RejectAttempt(ctx context.Context, value *federationdomain.Interaction) error {
	return changed(s.gate(s.db.WithContext(ctx), value).Where("stage IN ?", []string{StagePending, StageSecondFactor}).UpdateColumn("attempts", gorm.Expr("attempts + 1")))
}

func (s *Store) AwaitSecondFactor(ctx context.Context, value *federationdomain.Interaction, userID uint, version uint64, rememberMe bool) error {
	if userID == 0 {
		return ErrInteraction
	}
	return changed(s.gate(s.db.WithContext(ctx), value).Where("stage = ?", StagePending).Updates(map[string]any{"stage": StageSecondFactor, "pending_user_id": userID, "pending_token_version": version, "remember_me": rememberMe}))
}

func activeUser(db *gorm.DB, userID uint, version uint64) (*userdomain.User, error) {
	var user userdomain.User
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL AND status = ? AND token_version = ?", userID, constants.UserStatusActive, version).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	return &user, nil
}

func subjectFor(db *gorm.DB, userID uint, now time.Time) (string, error) {
	id, err := Secret()
	if err != nil {
		return "", err
	}
	value := federationdomain.Subject{ID: id, UserID: userID, CreatedAt: now}
	if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoNothing: true}).Create(&value).Error; err != nil {
		return "", err
	}
	// A populated primary key is implicitly added to GORM's WHERE clause. Read
	// into a fresh value so a concurrent/existing subject wins the unique key.
	var stored federationdomain.Subject
	if err := db.Where("user_id = ?", userID).First(&stored).Error; err != nil {
		return "", err
	}
	return stored.ID, nil
}

// AuthenticatePassword completes password-only authentication. Second-factor
// verification must use AuthenticateSecondFactor's transaction instead.
func (s *Store) AuthenticatePassword(ctx context.Context, value *federationdomain.Interaction, userID uint, version uint64, lifetime time.Duration, previousCookie string) (string, *federationdomain.BrowserSession, error) {
	return s.authenticate(ctx, value, userID, version, false, lifetime, nil, previousCookie)
}

// AuthenticateSecondFactor reserves an attempt under the same write transaction
// used by the real TOTP/recovery-code service and the new browser session. Failed
// proofs commit only the attempt; any later persistence failure rolls back proof
// consumption as well, so a recovery code is not lost without a session.
func (s *Store) AuthenticateSecondFactor(ctx context.Context, value *federationdomain.Interaction, lifetime time.Duration, verify func(*gorm.DB, uint) error, previousCookie string) (string, *federationdomain.BrowserSession, error) {
	if value == nil || value.Stage != StageSecondFactor || verify == nil {
		return "", nil, ErrInteraction
	}
	return s.authenticate(ctx, value, value.PendingUserID, value.PendingTokenVersion, true, lifetime, verify, previousCookie)
}

func (s *Store) authenticate(ctx context.Context, value *federationdomain.Interaction, userID uint, version uint64, secondFactor bool, lifetime time.Duration, verify func(*gorm.DB, uint) error, previousCookie string) (string, *federationdomain.BrowserSession, error) {
	if lifetime <= 0 || lifetime > 30*24*time.Hour || (previousCookie != "" && !validSecret(previousCookie)) {
		return "", nil, ErrInteraction
	}
	cookie, err := Secret()
	if err != nil {
		return "", nil, err
	}
	id, err := Secret()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	amr := "pwd"
	if secondFactor {
		amr = "pwd otp"
	}
	session := &federationdomain.BrowserSession{ID: id, CookieHash: Hash(cookie), UserID: userID, TokenVersion: version, AuthenticatedAt: now, ExpiresAt: now.Add(lifetime), AMR: amr}
	var proofErr error
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := activeUser(tx, userID, version)
		if err != nil {
			return err
		}
		if (user.TOTPEnabledAt != nil) != secondFactor {
			return ErrInteraction
		}
		query := s.gate(tx, value)
		if secondFactor {
			query = query.Where("stage = ? AND pending_user_id = ? AND pending_token_version = ?", StageSecondFactor, userID, version)
		} else {
			query = query.Where("stage = ?", StagePending)
		}
		if verify != nil {
			if err := changed(query.UpdateColumn("attempts", gorm.Expr("attempts + 1"))); err != nil {
				return err
			}
			if err := verify(tx, userID); err != nil {
				proofErr = err
				return nil
			}
			// The successful last attempt remains usable for protocol callbacks.
			query = s.boundGate(tx, value).Where("stage = ? AND pending_user_id = ? AND pending_token_version = ? AND attempts <= ?", StageSecondFactor, userID, version, MaxAttempts)
		}
		subject, err := subjectFor(tx, userID, now)
		if err != nil {
			return err
		}
		if err := changed(query.Updates(map[string]any{"stage": StageAuthenticated, "session_id": id, "subject_id": subject})); err != nil {
			return err
		}
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		// Rotate the current browser in the same commit as proof consumption and
		// session creation. A failed revocation must leave its recovery code usable.
		// Expired/revoked cookies are harmless; other devices are never selected.
		if previousCookie != "" {
			if err := tx.Model(&federationdomain.BrowserSession{}).Where("cookie_hash = ? AND revoked_at IS NULL", Hash(previousCookie)).UpdateColumn("revoked_at", now).Error; err != nil {
				return err
			}
		}
		return changed(tx.Model(&userdomain.User{}).Where("id = ? AND token_version = ?", userID, version).UpdateColumn("last_login_at", now))
	})
	if err != nil {
		return "", nil, err
	}
	if proofErr != nil {
		return "", nil, proofErr
	}
	return cookie, session, nil
}

func (s *Store) Session(ctx context.Context, cookie string) (*federationdomain.BrowserSession, error) {
	return s.session(s.db.WithContext(ctx), cookie, false)
}

func (s *Store) session(db *gorm.DB, cookie string, lock bool) (*federationdomain.BrowserSession, error) {
	if !validSecret(cookie) {
		return nil, ErrInteraction
	}
	var session federationdomain.BrowserSession
	query := db.Where("cookie_hash = ? AND revoked_at IS NULL AND expires_at > ?", Hash(cookie), s.now())
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	if err := sessionUser(db, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func sessionUser(db *gorm.DB, session *federationdomain.BrowserSession) error {
	user, err := activeUser(db, session.UserID, session.TokenVersion)
	if err != nil {
		return err
	}
	if user.TokenInvalidBefore != nil && !session.AuthenticatedAt.After(*user.TokenInvalidBefore) {
		return ErrInteraction
	}
	if user.TOTPEnabledAt != nil && session.AMR != "pwd otp" {
		return ErrInteraction
	}
	return nil
}

// ResumeSession associates a newly validated client request with the authority's
// existing browser session. All relying parties then share one revocable sid.
func (s *Store) ResumeSession(ctx context.Context, value *federationdomain.Interaction, cookie string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		session, err := s.session(tx, cookie, true)
		if err != nil {
			return err
		}
		subject, err := subjectFor(tx, session.UserID, s.now())
		if err != nil {
			return err
		}
		return changed(s.gate(tx, value).Where("stage = ?", StagePending).Updates(map[string]any{"stage": StageAuthenticated, "session_id": session.ID, "subject_id": subject}))
	})
}

func (s *Store) Cancel(ctx context.Context, value *federationdomain.Interaction) error {
	return changed(s.gate(s.db.WithContext(ctx), value).Where("stage IN ?", []string{StagePending, StageSecondFactor, StageAuthenticated, StageCodeIssued}).UpdateColumn("cancelled_at", s.now()))
}

func (s *Store) SaveCode(ctx context.Context, id, code string) error {
	if strings.TrimSpace(code) == "" {
		return ErrInteraction
	}
	return changed(s.db.WithContext(ctx).Model(&federationdomain.Interaction{}).Where("id = ? AND stage = ? AND expires_at > ? AND cancelled_at IS NULL AND code_hash IS NULL", id, StageAuthenticated, s.now()).Updates(map[string]any{"stage": StageCodeIssued, "code_hash": Hash(code), "code_expires_at": s.now().Add(90 * time.Second)}))
}

// ConsumeCode executes only after the OIDC library validates client, redirect
// and PKCE. Conditional consumption and access-token insertion share one commit.
func (s *Store) ConsumeCode(ctx context.Context, id, clientID, scopesJSON string) (*federationdomain.AccessToken, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(scopesJSON), &scopes); err != nil || len(scopes) == 0 || len(scopes) > 16 {
		return nil, ErrInteraction
	}
	var token *federationdomain.AccessToken
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request federationdomain.Interaction
		if err := tx.Where("id = ? AND client_id = ?", id, clientID).First(&request).Error; err != nil {
			return ErrInteraction
		}
		var session federationdomain.BrowserSession
		if err := tx.Where("id = ? AND revoked_at IS NULL AND expires_at > ?", request.SessionID, s.now()).First(&session).Error; err != nil {
			return ErrInteraction
		}
		if err := sessionUser(tx, &session); err != nil {
			return err
		}
		if err := changed(s.gate(tx, &request).Where("stage = ? AND code_hash IS NOT NULL AND code_expires_at > ?", StageCodeIssued, s.now()).UpdateColumn("code_consumed_at", s.now())); err != nil {
			return err
		}
		tokenID, err := Secret()
		if err != nil {
			return err
		}
		expiresAt := s.now().Add(5 * time.Minute)
		if session.ExpiresAt.Before(expiresAt) {
			expiresAt = session.ExpiresAt
		}
		token = &federationdomain.AccessToken{ID: tokenID, ClientID: clientID, SessionID: session.ID, SubjectID: request.SubjectID, ScopesJSON: scopesJSON, CreatedAt: s.now(), ExpiresAt: expiresAt}
		return tx.Create(token).Error
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (s *Store) RevokeSession(ctx context.Context, sessionID string) error {
	return s.db.WithContext(ctx).Model(&federationdomain.BrowserSession{}).Where("id = ? AND revoked_at IS NULL", sessionID).UpdateColumn("revoked_at", s.now()).Error
}

// Token checks the live authority session for every userinfo/introspection use;
// revoking a sid invalidates its tokens without waiting for their original TTL.
func (s *Store) Token(ctx context.Context, tokenID, clientID, subjectID string) (*federationdomain.AccessToken, error) {
	db := s.db.WithContext(ctx)
	var token federationdomain.AccessToken
	if err := db.Where("id = ? AND client_id = ? AND subject_id = ? AND revoked_at IS NULL AND expires_at > ?", tokenID, clientID, subjectID, s.now()).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	var session federationdomain.BrowserSession
	if err := db.Where("id = ? AND revoked_at IS NULL AND expires_at > ?", token.SessionID, s.now()).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	if err := sessionUser(db, &session); err != nil {
		return nil, err
	}
	return &token, nil
}

// Request is protocol-side lookup. Browser UI must use ReadBound instead.
func (s *Store) Request(ctx context.Context, id string) (*federationdomain.Interaction, error) {
	var value federationdomain.Interaction
	err := s.db.WithContext(ctx).Where("id = ? AND expires_at > ? AND cancelled_at IS NULL AND code_consumed_at IS NULL AND (attempts < ? OR stage IN ?)", id, s.now(), MaxAttempts, []string{StageAuthenticated, StageCodeIssued}).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInteraction
	}
	return &value, err
}

func (s *Store) RequestByCode(ctx context.Context, code string) (*federationdomain.Interaction, error) {
	if code == "" {
		return nil, ErrInteraction
	}
	var value federationdomain.Interaction
	err := s.db.WithContext(ctx).Where("code_hash = ? AND stage = ? AND expires_at > ? AND code_expires_at > ? AND cancelled_at IS NULL AND code_consumed_at IS NULL", Hash(code), StageCodeIssued, s.now(), s.now()).First(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInteraction
	}
	return &value, err
}

func (s *Store) SessionByID(ctx context.Context, id string) (*federationdomain.BrowserSession, error) {
	db := s.db.WithContext(ctx)
	var session federationdomain.BrowserSession
	if err := db.Where("id = ? AND revoked_at IS NULL AND expires_at > ?", id, s.now()).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	if err := sessionUser(db, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) UserForSubject(ctx context.Context, subject string) (*userdomain.User, error) {
	db := s.db.WithContext(ctx)
	var value federationdomain.Subject
	if err := db.First(&value, "id = ?", subject).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	var user userdomain.User
	if err := db.Where("id = ? AND deleted_at IS NULL AND status = ?", value.UserID, constants.UserStatusActive).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	return &user, nil
}

// LiveToken identifies its registered client from the unguessable token ID,
// then applies the same live-session checks used for introspection.
func (s *Store) LiveToken(ctx context.Context, id, subject string) (*federationdomain.AccessToken, error) {
	var token federationdomain.AccessToken
	if err := s.db.WithContext(ctx).First(&token, "id = ? AND subject_id = ?", id, subject).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInteraction
		}
		return nil, err
	}
	return s.Token(ctx, id, token.ClientID, subject)
}

func (s *Store) RevokeToken(ctx context.Context, id, clientID, subject string) error {
	return s.db.WithContext(ctx).Model(&federationdomain.AccessToken{}).Where("id = ? AND client_id = ? AND subject_id = ? AND revoked_at IS NULL", id, clientID, subject).UpdateColumn("revoked_at", s.now()).Error
}

// FinishRequest is idempotent after the atomic token commit. It deliberately
// retains the consumed record for replay rejection and bounded later cleanup.
func (s *Store) FinishRequest(ctx context.Context, id string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&federationdomain.Interaction{}).Where("id = ? AND code_consumed_at IS NOT NULL", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrInteraction
	}
	return nil
}

func (s *Store) Health(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec("SELECT 1").Error
}
