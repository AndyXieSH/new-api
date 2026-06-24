package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestEmailVerificationCodePersistsHashAndConsumesOnce(t *testing.T) {
	truncateTables(t)

	email := "USER@example.com"
	code := "123456"
	require.NoError(t, CreateEmailVerificationCode(email, common.EmailVerificationPurpose, code, "127.0.0.1", 10))

	var stored EmailVerification
	require.NoError(t, DB.First(&stored, "email = ?", "user@example.com").Error)
	require.NotEqual(t, code, stored.CodeHash)
	require.NotEmpty(t, stored.CodeHash)
	require.Equal(t, "127.0.0.1", stored.SendIP)

	ok, err := VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, code)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, code)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEmailVerificationCodeRejectsExpiredCode(t *testing.T) {
	truncateTables(t)

	email := "expired@example.com"
	code := "654321"
	require.NoError(t, CreateEmailVerificationCode(email, common.EmailVerificationPurpose, code, "", 10))
	require.NoError(t, DB.Model(&EmailVerification{}).
		Where("email = ?", email).
		Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)

	ok, err := VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, code)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestEmailVerificationCodeLatestSendInvalidatesPreviousCode(t *testing.T) {
	truncateTables(t)

	email := "latest@example.com"
	require.NoError(t, CreateEmailVerificationCode(email, common.EmailVerificationPurpose, "111111", "", 10))
	require.NoError(t, CreateEmailVerificationCode(email, common.EmailVerificationPurpose, "222222", "", 10))

	ok, err := VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, "111111")
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, "222222")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestEmailVerificationCodeStopsAfterMaxAttempts(t *testing.T) {
	truncateTables(t)

	email := "attempts@example.com"
	code := "999999"
	require.NoError(t, CreateEmailVerificationCode(email, common.EmailVerificationPurpose, code, "", 10))

	for i := 0; i < EmailVerificationMaxAttempts; i++ {
		ok, err := VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, "000000")
		require.NoError(t, err)
		require.False(t, ok)
	}

	ok, err := VerifyEmailVerificationCode(email, common.EmailVerificationPurpose, code)
	require.NoError(t, err)
	require.False(t, ok)
}
