// Package federationdomain owns the durable records used by the OIDC adapter.
// It does not grant access or merge accounts by email.
package federationdomain

import (
	"errors"
	"time"
)

var ErrInteraction = errors.New("identity interaction unavailable")

const (
	StagePending       = "pending"
	StageSecondFactor  = "second_factor"
	StageAuthenticated = "authenticated"
	StageCodeIssued    = "code_issued"
)

// Subject is immutable, opaque, and independent of changeable email/profile data.
type Subject struct {
	ID        string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Subject) TableName() string { return "identity_subjects" }

// BrowserSession is an authority session shared through OIDC, never a parent-
// domain authentication cookie. Only the hash of its cookie secret is stored.
type BrowserSession struct {
	ID              string     `gorm:"primaryKey;size:64"`
	CookieHash      string     `gorm:"not null;uniqueIndex;size:64" json:"-"`
	UserID          uint       `gorm:"not null;index"`
	TokenVersion    uint64     `gorm:"not null"`
	AuthenticatedAt time.Time  `gorm:"not null"`
	ExpiresAt       time.Time  `gorm:"not null;index"`
	RevokedAt       *time.Time `gorm:"index"`
	AMR             string     `gorm:"not null"`
}

func (BrowserSession) TableName() string { return "identity_browser_sessions" }

// Interaction stores the protocol request, browser binding and monotonic login
// state. Raw cookie, password, verification code and TOTP values never belong here.
type Interaction struct {
	ID                  string    `gorm:"primaryKey;size:64"`
	ClientID            string    `gorm:"not null;index;size:128"`
	RequestJSON         string    `gorm:"not null;type:text" json:"-"`
	BrowserHash         string    `gorm:"not null;size:64" json:"-"`
	CSRFHash            string    `gorm:"not null;size:64" json:"-"`
	CreatedAt           time.Time `gorm:"not null"`
	ExpiresAt           time.Time `gorm:"not null;index"`
	Stage               string    `gorm:"not null;size:32"`
	PendingUserID       uint      `gorm:"not null;default:0"`
	PendingTokenVersion uint64    `gorm:"not null;default:0"`
	RememberMe          bool      `gorm:"not null;default:false"`
	Attempts            uint      `gorm:"not null;default:0"`
	SessionID           string    `gorm:"not null;default:'';size:64"`
	SubjectID           string    `gorm:"not null;default:'';size:64"`
	// A nullable unique hash allows many requests that have not issued a code.
	CodeHash       *string `gorm:"uniqueIndex;size:64" json:"-"`
	CodeExpiresAt  *time.Time
	CodeConsumedAt *time.Time
	CancelledAt    *time.Time
}

func (Interaction) TableName() string { return "identity_interactions" }

type AccessToken struct {
	ID         string    `gorm:"primaryKey;size:64"`
	ClientID   string    `gorm:"not null;index;size:128"`
	SessionID  string    `gorm:"not null;index;size:64"`
	SubjectID  string    `gorm:"not null;size:64"`
	ScopesJSON string    `gorm:"not null;type:text"`
	CreatedAt  time.Time `gorm:"not null"`
	ExpiresAt  time.Time `gorm:"not null;index"`
	RevokedAt  *time.Time
}

func (AccessToken) TableName() string { return "identity_access_tokens" }

// ClientSession records a delegation established by a consumed OIDC code. It
// authorizes confidential-client session checks, independently of short-lived
// API tokens. A sid alone never grants this delegation to another client.
type ClientSession struct {
	ClientID  string    `gorm:"primaryKey;size:128"`
	SessionID string    `gorm:"primaryKey;size:64"`
	SubjectID string    `gorm:"not null;size:64"`
	CreatedAt time.Time `gorm:"not null"`
}

func (ClientSession) TableName() string { return "identity_client_sessions" }
