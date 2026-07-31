//go:build redis_integration

package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"admin/dto"
	"admin/global"
	"admin/initialize/testutil"
	"admin/middleware"
	"admin/model"
	"admin/service"
	"admin/utils"
)

func TestRealRedisAccessRefreshAndBlacklistCombination(t *testing.T) {
	const (
		accessExpireMins  = 15
		refreshExpireMins = 60
		accessVersion     = 7
	)

	ctx := context.Background()
	redisClient := openRealRedisFinalGate(t)
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserAccessVersion{},
	); err != nil {
		t.Fatalf("migrate isolated token database: %v", err)
	}

	user := model.User{
		Username: "real-redis-final-gate",
		Password: "not-used",
		Email:    "real-redis-final-gate@example.test",
		Role:     "user",
		Status:   1,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create real Redis final-gate user: %v", err)
	}
	if err := db.Create(&model.UserAccessVersion{
		UserID:  user.ID,
		Version: accessVersion,
	}).Error; err != nil {
		t.Fatalf("create real Redis final-gate access version: %v", err)
	}

	previousDB := global.DB
	previousRedis := global.Redis
	global.DB = db
	global.Redis = redisClient
	t.Cleanup(func() {
		global.DB = previousDB
		global.Redis = previousRedis
		_ = redisClient.Close()
	})

	secret := fmt.Sprintf("real-redis-final-gate-%d", time.Now().UnixNano())
	cfg := service.JWTConfig{
		Secret:            secret,
		ExpireMins:        accessExpireMins,
		RefreshExpireMins: refreshExpireMins,
	}
	accessToken, refreshToken, err := utils.GenerateToken(
		user.ID,
		accessVersion,
		[]string{"user"},
		[]string{"profile.read"},
		secret,
		accessExpireMins,
		refreshExpireMins,
	)
	if err != nil {
		t.Fatalf("generate real Redis token pair: %v", err)
	}
	blacklistKeys := []string{
		"blacklist:" + accessToken,
		"blacklist:" + refreshToken,
	}
	t.Cleanup(func() {
		if err := redisClient.Del(ctx, blacklistKeys...).Err(); err != nil {
			t.Errorf("delete real Redis final-gate keys: %v", err)
		}
	})

	router := gin.New()
	router.GET("/protected", middleware.JWTAuth(cfg), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": http.StatusOK})
	})

	assertProtectedStatus(t, router, accessToken, http.StatusOK)

	if _, err := service.RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: refreshToken,
	}, cfg); err != nil {
		t.Fatalf("initial RefreshTokens() error = %v", err)
	}

	service.Logout(accessToken, accessExpireMins)
	assertRealRedisBlacklist(
		t,
		redisClient,
		blacklistKeys[0],
		time.Duration(accessExpireMins)*time.Minute,
	)
	assertProtectedStatus(t, router, accessToken, http.StatusUnauthorized)

	if _, err := service.RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: refreshToken,
	}, cfg); err != nil {
		t.Fatalf(
			"RefreshTokens() after access-token blacklist error = %v",
			err,
		)
	}

	service.Logout(refreshToken, refreshExpireMins)
	assertRealRedisBlacklist(
		t,
		redisClient,
		blacklistKeys[1],
		time.Duration(refreshExpireMins)*time.Minute,
	)
	if _, err := service.RefreshTokens(dto.RefreshTokenReq{
		RefreshToken: refreshToken,
	}, cfg); !errors.Is(err, service.ErrRefreshTokenInvalid) {
		t.Fatalf(
			"RefreshTokens() after refresh-token blacklist error = %v, want %v",
			err,
			service.ErrRefreshTokenInvalid,
		)
	}
}

func openRealRedisFinalGate(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("ADMIN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Fatal("ADMIN_TEST_REDIS_ADDR is required for the real Redis gate")
	}
	database := 15
	if rawDatabase := os.Getenv("ADMIN_TEST_REDIS_DB"); rawDatabase != "" {
		parsed, err := strconv.Atoi(rawDatabase)
		if err != nil {
			t.Fatalf("parse ADMIN_TEST_REDIS_DB: %v", err)
		}
		database = parsed
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     os.Getenv("ADMIN_TEST_REDIS_PASSWORD"),
		DB:           database,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		t.Fatalf("ping real Redis at %s: %v", addr, err)
	}
	return client
}

func assertProtectedStatus(
	t *testing.T,
	handler http.Handler,
	token string,
	wantStatus int,
) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf(
			"protected request status = %d, want %d; body=%s",
			recorder.Code,
			wantStatus,
			recorder.Body.String(),
		)
	}
}

func assertRealRedisBlacklist(
	t *testing.T,
	client *redis.Client,
	key string,
	maxTTL time.Duration,
) {
	t.Helper()

	ctx := context.Background()
	value, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("read real Redis blacklist key: %v", err)
	}
	if value != "1" {
		t.Fatalf("real Redis blacklist value = %q, want 1", value)
	}
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("read real Redis blacklist TTL: %v", err)
	}
	if ttl <= 0 || ttl > maxTTL {
		t.Fatalf(
			"real Redis blacklist TTL = %s, want positive and <= %s",
			ttl,
			maxTTL,
		)
	}
}
