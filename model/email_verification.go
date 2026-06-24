package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const EmailVerificationMaxAttempts = 5

type EmailVerification struct {
	Id           int    `json:"id"`
	Email        string `json:"email" gorm:"type:varchar(191);index:idx_email_verification_lookup,priority:1;not null"`
	Purpose      string `json:"purpose" gorm:"type:varchar(16);index:idx_email_verification_lookup,priority:2;not null"`
	CodeHash     string `json:"-" gorm:"type:varchar(128);not null"`
	ExpiresAt    int64  `json:"expires_at" gorm:"index:idx_email_verification_lookup,priority:3;not null"`
	UsedAt       int64  `json:"used_at" gorm:"index;default:0"`
	AttemptCount int    `json:"attempt_count" gorm:"type:int;default:0"`
	SendIP       string `json:"send_ip" gorm:"type:varchar(64);default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

func normalizeVerificationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CreateEmailVerificationCode(email string, purpose string, code string, sendIP string, validMinutes int) error {
	email = normalizeVerificationEmail(email)
	purpose = strings.TrimSpace(purpose)
	code = strings.TrimSpace(code)
	if email == "" || purpose == "" || code == "" {
		return errors.New("email verification code params cannot be empty")
	}
	if validMinutes <= 0 {
		validMinutes = common.VerificationValidMinutes
	}
	codeHash, err := common.Password2Hash(code)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&EmailVerification{}).
			Where("email = ? AND purpose = ? AND used_at = 0", email, purpose).
			Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Create(&EmailVerification{
			Email:     email,
			Purpose:   purpose,
			CodeHash:  codeHash,
			ExpiresAt: now + int64(validMinutes)*60,
			SendIP:    strings.TrimSpace(sendIP),
		}).Error
	})
}

func VerifyEmailVerificationCode(email string, purpose string, code string) (bool, error) {
	email = normalizeVerificationEmail(email)
	purpose = strings.TrimSpace(purpose)
	code = strings.TrimSpace(code)
	if email == "" || purpose == "" || code == "" {
		return false, nil
	}
	now := time.Now().Unix()
	verified := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var record EmailVerification
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("email = ? AND purpose = ? AND used_at = 0 AND expires_at > ?", email, purpose, now).
			Order("id desc").
			First(&record).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if record.AttemptCount >= EmailVerificationMaxAttempts {
			return nil
		}
		record.AttemptCount++
		if !common.ValidatePasswordAndHash(code, record.CodeHash) {
			return tx.Model(&EmailVerification{}).Where("id = ?", record.Id).Update("attempt_count", record.AttemptCount).Error
		}
		verified = true
		return tx.Model(&EmailVerification{}).
			Where("id = ? AND used_at = 0", record.Id).
			Updates(map[string]interface{}{
				"used_at":       now,
				"attempt_count": record.AttemptCount,
			}).Error
	})
	return verified, err
}
