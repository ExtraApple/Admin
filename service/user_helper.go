package service

import (
	"errors"
	"unicode"
)

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
