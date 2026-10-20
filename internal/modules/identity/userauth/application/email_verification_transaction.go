package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"sort"
	"strings"
	"time"

	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
)

var ErrAuthTransactionUnavailable = errors.New("user authentication transaction is not configured")

type emailProof struct {
	email, purpose, code string
}

// withEmailProofs commits a rejected attempt's counter, but consumes successful
// proofs only in the same transaction as the protected account mutation. For a
// two-email change, all proofs are checked before any of them is consumed.
func (s *Service) withEmailProofs(proofs []emailProof, mutate func(AuthTransaction) error) error {
	return s.emailProofTransaction(proofs, true, mutate)
}

// checkEmailProofs rejects guesses cheaply before bcrypt, without consuming a
// valid proof or holding database locks during password hashing. The committing
// transaction always checks the proofs again.
func (s *Service) checkEmailProofs(proofs []emailProof) error {
	if len(proofs) == 0 {
		return nil
	}
	return s.emailProofTransaction(proofs, false, nil)
}

func (s *Service) emailProofTransaction(proofs []emailProof, consume bool, mutate func(AuthTransaction) error) error {
	if s.authUnitOfWork == nil {
		return ErrAuthTransactionUnavailable
	}
	// A stable lock order also covers concurrent two-address operations.
	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].email == proofs[j].email {
			return proofs[i].purpose < proofs[j].purpose
		}
		return proofs[i].email < proofs[j].email
	})
	var rejection error
	err := s.authUnitOfWork.WithinTransaction(context.Background(), func(tx AuthTransaction) error {
		// The unit of work may retry after a serialization/deadlock failure.
		rejection = nil
		now := time.Now()
		maxAttempts := resolveMaxAttempts(s.cfg.Email.VerifyCode)
		records := make([]*emailverificationdomain.Code, 0, len(proofs))
		for _, proof := range proofs {
			record, err := tx.GetLatestVerificationCodeForUpdate(proof.email, proof.purpose)
			if err != nil {
				return err
			}
			switch {
			case record == nil || record.VerifiedAt != nil:
				rejection = ErrVerifyCodeInvalid
			case !record.ExpiresAt.After(now):
				rejection = ErrVerifyCodeExpired
			case maxAttempts > 0 && record.AttemptCount >= maxAttempts:
				rejection = ErrVerifyCodeAttemptsExceeded
			case subtle.ConstantTimeCompare([]byte(strings.TrimSpace(record.Code)), []byte(strings.TrimSpace(proof.code))) != 1:
				if err := tx.IncrementVerificationAttempt(record.ID, now, maxAttempts); err != nil {
					return err
				}
				rejection = ErrVerifyCodeInvalid
			}
			if rejection != nil {
				// Returning nil commits only the failed-attempt counter. The
				// caller still receives rejection and no mutation is executed.
				return nil
			}
			records = append(records, record)
		}
		if !consume {
			return nil
		}
		for _, record := range records {
			consumed, err := tx.ConsumeVerificationCode(record.ID, now, maxAttempts)
			if err != nil {
				return err
			}
			if !consumed {
				// This error rolls back ANY proofs already consumed in this
				// transaction; a successful proof is never partially spent.
				return ErrVerifyCodeInvalid
			}
		}
		return mutate(tx)
	})
	if err != nil {
		return err
	}
	return rejection
}
