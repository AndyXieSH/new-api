package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type emailVerificationAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func setupEmailVerificationControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.EmailVerificationEnabled = true
	constant.GenerateDefaultToken = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Log{}, &model.EmailVerification{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}

	t.Cleanup(func() {
		common.EmailVerificationEnabled = false
		constant.GenerateDefaultToken = false
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newEmailVerificationContext(t *testing.T, method string, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func decodeEmailVerificationResponse(t *testing.T, recorder *httptest.ResponseRecorder) emailVerificationAPIResponse {
	t.Helper()

	var response emailVerificationAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return response
}

func TestRegisterConsumesPersistedEmailVerificationCode(t *testing.T) {
	setupEmailVerificationControllerTestDB(t)
	requireCode := "123456"
	requireEmail := "new-user@example.com"
	if err := model.CreateEmailVerificationCode(requireEmail, common.EmailVerificationPurpose, requireCode, "127.0.0.1", 10); err != nil {
		t.Fatalf("failed to create verification code: %v", err)
	}

	body := map[string]any{
		"username":          "newuser",
		"password":          "password123",
		"email":             strings.ToUpper(requireEmail),
		"verification_code": requireCode,
	}
	ctx, recorder := newEmailVerificationContext(t, http.MethodPost, "/api/user/register", body)
	Register(ctx)

	response := decodeEmailVerificationResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected register success, got message: %s", response.Message)
	}

	var user model.User
	if err := model.DB.First(&user, "username = ?", "newuser").Error; err != nil {
		t.Fatalf("failed to load created user: %v", err)
	}
	if user.Email != requireEmail {
		t.Fatalf("expected normalized email %q, got %q", requireEmail, user.Email)
	}

	ctx, recorder = newEmailVerificationContext(t, http.MethodPost, "/api/user/register", map[string]any{
		"username":          "newuser2",
		"password":          "password123",
		"email":             "new-user2@example.com",
		"verification_code": requireCode,
	})
	Register(ctx)
	response = decodeEmailVerificationResponse(t, recorder)
	if response.Success {
		t.Fatalf("expected reused verification code to fail")
	}
}

func TestResetPasswordConsumesPersistedResetToken(t *testing.T) {
	setupEmailVerificationControllerTestDB(t)

	user := model.User{
		Username:    "resetuser",
		Password:    "oldpass123",
		DisplayName: "resetuser",
		Email:       "reset@example.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	if err := user.Insert(0); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	token := "reset-token"
	if err := model.CreateEmailVerificationCode(user.Email, common.PasswordResetPurpose, token, "127.0.0.1", 10); err != nil {
		t.Fatalf("failed to create reset token: %v", err)
	}

	ctx, recorder := newEmailVerificationContext(t, http.MethodPost, "/api/user/reset", map[string]any{
		"email": user.Email,
		"token": token,
	})
	ResetPassword(ctx)

	response := decodeEmailVerificationResponse(t, recorder)
	if !response.Success || response.Data == "" {
		t.Fatalf("expected password reset success, got success=%v message=%s data=%q", response.Success, response.Message, response.Data)
	}

	var reloaded model.User
	if err := model.DB.First(&reloaded, "email = ?", user.Email).Error; err != nil {
		t.Fatalf("failed to reload user: %v", err)
	}
	if !common.ValidatePasswordAndHash(response.Data, reloaded.Password) {
		t.Fatalf("reset response password does not match stored password hash")
	}

	ctx, recorder = newEmailVerificationContext(t, http.MethodPost, "/api/user/reset", map[string]any{
		"email": user.Email,
		"token": token,
	})
	ResetPassword(ctx)
	response = decodeEmailVerificationResponse(t, recorder)
	if response.Success {
		t.Fatalf("expected reused reset token to fail")
	}
}
