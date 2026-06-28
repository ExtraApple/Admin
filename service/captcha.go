package service

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"

	"admin/global"
)

// ========== Redis 存储适配 ==========

// redisStore 实现 base64Captcha.Store 接口，将验证码答案存入 Redis。
//
//	key 格式:  captcha:<id>
//	过期策略:  5 分钟自动过期（防 Redis 膨胀）
//	校验策略:  一次性消费，验证后立即删除（防重复使用）
type redisStore struct{}

// Set 将验证码答案存入 Redis。
//
//	参数 id:    验证码唯一标识
//	参数 value: 正确答案（如 "385210"）
//	返回值:     成功返回 nil，Redis 连接失败返回 error
func (s *redisStore) Set(id string, value string) error {
	ctx := context.Background()
	return global.Redis.Set(ctx, "captcha:"+id, value, 5*time.Minute).Err()
}

// Get 从 Redis 读取验证码答案。
//
//	参数 id:    验证码唯一标识
//	参数 clear: 读取后是否立即删除（通常校验场景为 true，防重复提交）
//	返回值:     存在则返回答案字符串，不存在/过期返回空串 ""
func (s *redisStore) Get(id string, clear bool) string {
	ctx := context.Background()
	key := "captcha:" + id
	val, err := global.Redis.Get(ctx, key).Result()
	if err != nil {
		return ""
	}
	if clear {
		global.Redis.Del(ctx, key)
	}
	return val
}

// Verify 校验用户输入的验证码是否正确。
//
//	参数 id:     验证码唯一标识
//	参数 answer: 用户输入的答案
//	参数 clear:  校验后是否删除（true = 一次性消费，校验通过立即销毁）
//	返回值:      匹配成功返回 true，错误 / 过期 / 不匹配返回 false
func (s *redisStore) Verify(id string, answer string, clear bool) bool {
	return s.Get(id, clear) == answer
}

// ========== 验证码服务 ==========

// captchaDriver 全局验证码图片生成器，只实例化一次。
//
//	参数说明（对应 NewDriverDigit 构造器）:
//	  高度 80px / 宽度 240px —— 横版长条，前端排版友好
//	  长度 6 位数字 —— 足够防止暴力破解（1/1,000,000）
//	  倾斜系数 0.7  —— 字符适当扭曲但不影响人眼识别
//	  噪点密度 80    —— 足够阻挡 OCR 扫描，不干扰正常阅读
var captchaDriver = base64Captcha.NewDriverDigit(80, 240, 6, 0.7, 80)

// GenerateCaptcha 生成 6 位数字图片验证码。
//
// 流程:
//
//  1. 创建 Redis 存储实例（实现 base64Captcha.Store 接口）
//
//  2. 调用 base64Captcha 生成器产出图片 + 答案
//
//  3. 答案自动写入 Redis（key = "captcha:<id>"，5 分钟过期）
//
//     返回值 id:   验证码唯一标识（UUID，前端提交时回传）
//     返回值 b64s: base64 编码的 PNG 图片（可直接放入 <img src>）
//     返回值 err:  生成失败时非 nil（通常是字体/绘图异常）
func GenerateCaptcha() (id string, b64s string, err error) {
	store := &redisStore{}
	c := base64Captcha.NewCaptcha(captchaDriver, store)
	id, b64s, _, err = c.Generate()
	return
}

// VerifyCaptcha 校验验证码并立即删除（一次性消费）。
//
// 设计意图:
//
//   - 每个验证码只能使用一次，防止同一个 captcha_id 被重复提交
//
//   - 即使验证失败也会删除，防止暴力穷举
//
//     参数 id:     前端传回的 captcha_id
//     参数 answer: 用户输入的 6 位数字
//     返回值:      匹配成功返回 true
func VerifyCaptcha(id string, answer string) bool {
	store := &redisStore{}
	return store.Verify(id, answer, true)
}
