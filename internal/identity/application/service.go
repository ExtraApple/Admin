package application

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode"

	"admin/internal/identity/domain"
)

const maxLoginFailures = 5

var (
	ErrCaptchaInvalid         = errors.New("验证码错误或已过期")
	ErrRefreshTokenInvalid    = errors.New("Refresh Token 无效或已过期")
	ErrAvatarFieldNotWritable = errors.New("头像只能通过专用接口修改")
	ErrUserNotFound           = errors.New("用户不存在")
)

type LoginRequest struct {
	Username    string
	Password    string
	CaptchaID   string
	CaptchaCode string
}

type RegisterRequest struct {
	Username    string
	Password    string
	Email       string
	Nickname    string
	CaptchaID   string
	CaptchaCode string
}

type RefreshRequest struct{ RefreshToken string }

type TokenIssue struct {
	UserID       uint
	TokenVersion int
	Roles        []string
	Permissions  []string
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenPurpose string

const (
	TokenPurposeAccess  TokenPurpose = "access"
	TokenPurposeRefresh TokenPurpose = "refresh"
)

type TokenClaims struct {
	UserID       uint
	TokenVersion int
	Purpose      TokenPurpose
}

type AuthenticatedIdentity struct {
	UserID      uint
	Roles       []string
	Permissions []string
}
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	User         domain.User
}

// Service owns authentication and token lifecycle use cases without exposing
// persistence, Redis, JWT, or HTTP implementation details.
type Service struct {
	users         UserRepository
	captcha       CaptchaVerifier
	passwords     PasswordHasher
	attempts      LoginAttemptStore
	blacklist     BlacklistStore
	authorization AuthorizationReader
	tokens        TokenService
}

func NewService(
	users UserRepository,
	captcha CaptchaVerifier,
	passwords PasswordHasher,
	attempts LoginAttemptStore,
	blacklist BlacklistStore,
	authorization AuthorizationReader,
	tokens TokenService,
) *Service {
	return &Service{users: users, captcha: captcha, passwords: passwords, attempts: attempts, blacklist: blacklist, authorization: authorization, tokens: tokens}
}

func (service *Service) Register(ctx context.Context, request RegisterRequest) (domain.User, error) {
	if !service.captcha.Verify(ctx, request.CaptchaID, request.CaptchaCode) {
		return domain.User{}, ErrCaptchaInvalid
	}
	if err := validatePassword(request.Password); err != nil {
		return domain.User{}, err
	}
	exists, err := service.users.ExistsByUsernameOrEmail(ctx, request.Username, request.Email)
	if err != nil {
		return domain.User{}, fmt.Errorf("查询用户失败: %w", err)
	}
	if exists {
		return domain.User{}, errors.New("用户名或邮箱已被注册")
	}
	hashed, err := service.passwords.Hash(request.Password)
	if err != nil {
		return domain.User{}, errors.New("密码加密失败")
	}
	user := domain.User{Username: request.Username, Password: hashed, Email: request.Email, Nickname: request.Nickname, Role: "user", Status: 1}
	if err := service.users.Create(ctx, &user); err != nil {
		return domain.User{}, fmt.Errorf("创建用户失败: %w", err)
	}
	return user, nil
}

func (service *Service) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	if !service.captcha.Verify(ctx, request.CaptchaID, request.CaptchaCode) {
		return LoginResult{}, ErrCaptchaInvalid
	}
	if remaining, locked, err := service.attempts.IsLocked(ctx, request.Username); err != nil {
		return LoginResult{}, err
	} else if locked {
		return LoginResult{}, fmt.Errorf("账号已被锁定，请 %d 分钟后重试", int(remaining.Minutes())+1)
	}
	user, err := service.users.FindByUsername(ctx, request.Username)
	if err != nil {
		return LoginResult{}, errors.New("用户名或密码错误")
	}
	if !user.Enabled() {
		return LoginResult{}, errors.New("账号已被禁用")
	}
	if err := service.passwords.Compare(user.Password, request.Password); err != nil {
		failures, recordErr := service.attempts.RecordFailure(ctx, request.Username)
		if recordErr != nil {
			return LoginResult{}, recordErr
		}
		if lockDuration := lockDurationForFailures(failures); lockDuration > 0 {
			if err := service.attempts.Lock(ctx, request.Username, lockDuration); err != nil {
				return LoginResult{}, err
			}
		}
		return LoginResult{}, fmt.Errorf("用户名或密码错误（剩余尝试: %d 次）", maxLoginFailures-failures)
	}

	version, err := service.authorization.EnsureVersion(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("初始化用户授权版本失败: %w", err)
	}
	snapshot, err := service.authorization.Snapshot(ctx, user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if snapshot.Version != version {
		return LoginResult{}, errors.New("用户授权版本不一致")
	}
	tokens, err := service.tokens.Issue(ctx, TokenIssue{UserID: user.ID, TokenVersion: version, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)})
	if err != nil {
		return LoginResult{}, fmt.Errorf("生成 Token 失败: %w", err)
	}
	if err := service.attempts.Clear(ctx, request.Username); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, User: user}, nil
}

func (service *Service) Refresh(ctx context.Context, request RefreshRequest) (TokenPair, error) {
	claims, err := service.tokens.Parse(ctx, request.RefreshToken)
	if err != nil || claims.Purpose != TokenPurposeRefresh {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	blacklisted, err := service.blacklist.Contains(ctx, request.RefreshToken)
	if err != nil || blacklisted {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	user, err := service.users.FindByID(ctx, claims.UserID)
	if err != nil || !user.Enabled() {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	snapshot, err := service.authorization.Snapshot(ctx, claims.UserID)
	if err != nil || claims.TokenVersion <= 0 || claims.TokenVersion != snapshot.Version {
		return TokenPair{}, ErrRefreshTokenInvalid
	}
	return service.tokens.Issue(ctx, TokenIssue{UserID: claims.UserID, TokenVersion: snapshot.Version, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)})
}

func (service *Service) Authenticate(ctx context.Context, token string) (AuthenticatedIdentity, error) {
	claims, err := service.tokens.Parse(ctx, token)
	if err != nil || claims.Purpose != TokenPurposeAccess {
		return AuthenticatedIdentity{}, errors.New("Token 无效或已过期")
	}
	blacklisted, err := service.blacklist.Contains(ctx, token)
	if err != nil || blacklisted {
		return AuthenticatedIdentity{}, errors.New("Token 已失效")
	}
	user, err := service.users.FindByID(ctx, claims.UserID)
	if err != nil || !user.Enabled() {
		return AuthenticatedIdentity{}, errors.New("账号已被禁用")
	}
	snapshot, err := service.authorization.Snapshot(ctx, claims.UserID)
	if err != nil || claims.TokenVersion <= 0 || claims.TokenVersion != snapshot.Version {
		return AuthenticatedIdentity{}, errors.New("Token已失效，请重新登录")
	}
	return AuthenticatedIdentity{UserID: claims.UserID, Roles: cloneStrings(snapshot.Roles), Permissions: cloneStrings(snapshot.Permissions)}, nil
}

func (service *Service) Logout(ctx context.Context, token string, expire time.Duration) error {
	return service.blacklist.Add(ctx, token, expire)
}

func lockDurationForFailures(failures int) time.Duration {
	switch {
	case failures < maxLoginFailures:
		return 0
	case failures < 10:
		return time.Minute
	case failures < 15:
		return 5 * time.Minute
	case failures < 20:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func validatePassword(password string) error {
	if len(password) < 6 {
		return errors.New("密码长度不能少于 6 位")
	}
	var upper, lower, digit, special bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			special = true
		}
	}
	count := 0
	for _, present := range []bool{upper, lower, digit, special} {
		if present {
			count++
		}
	}
	if count < 3 {
		return errors.New("密码必须包含大写字母、小写字母、数字、特殊符号中至少 3 种")
	}
	return nil
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }
