package service

import (
	"context"
	"errors"
	"time"
	"unicode"

	"admin/global"
)

const maxFailures = 5

// validatePassword 校验密码复杂度：至少 6 位，并包含至少 3 种字符类型。
func validatePassword(pw string) error {
	if len(pw) < 6 {
		return errors.New("密码长度不能少于 6 位")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	count := 0
	for _, ok := range []bool{hasUpper, hasLower, hasDigit, hasSpecial} {
		if ok {
			count++
		}
	}
	if count < 3 {
		return errors.New("密码必须包含大写字母、小写字母、数字、特殊符号中至少 3 种")
	}
	return nil
}

func lockDuration(failures int) int {
	switch {
	case failures < maxFailures:
		return 0
	case failures < 10:
		return 1
	case failures < 15:
		return 5
	case failures < 20:
		return 15
	default:
		return 60
	}
}

func isLocked(username string) (int, bool) {
	val, err := global.Redis.Get(context.Background(), "lock:"+username).Result()
	if err != nil || val != "1" {
		return 0, false
	}

	ttl, _ := global.Redis.TTL(context.Background(), "lock:"+username).Result()
	mins := int(ttl.Minutes()) + 1
	return mins, true
}

func incrFailed(username string) int {
	key := "fail:" + username
	count, _ := global.Redis.Incr(context.Background(), key).Result()
	global.Redis.Expire(context.Background(), key, 24*time.Hour)
	return int(count)
}

func lockAfterFail(username string, failures int) {
	dur := lockDuration(failures)
	if dur == 0 {
		return
	}

	global.Redis.Set(context.Background(), "lock:"+username, "1", time.Duration(dur)*time.Minute)
	if failures >= 10 {
		global.Redis.Del(context.Background(), "fail:"+username)
	}
}

func clearFailed(username string) {
	global.Redis.Del(context.Background(), "fail:"+username, "lock:"+username)
}
