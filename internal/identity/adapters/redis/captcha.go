package redisadapter

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"
	"github.com/redis/go-redis/v9"
)

type captchaRedisStore struct{ client *redis.Client }

func (store captchaRedisStore) Set(id, value string) error {
	return store.client.Set(context.Background(), captchaPrefix+id, value, 5*time.Minute).Err()
}

func (store captchaRedisStore) Get(id string, clear bool) string {
	value, err := store.client.Get(context.Background(), captchaPrefix+id).Result()
	if err != nil { return "" }
	if clear { _ = store.client.Del(context.Background(), captchaPrefix+id).Err() }
	return value
}

func (store captchaRedisStore) Verify(id, answer string, clear bool) bool {
	value := store.Get(id, clear)
	return value == answer
}

var captchaDriver = base64Captcha.NewDriverDigit(80, 240, 6, 0.7, 80)

func (store *Store) GenerateCaptcha() (id, image string, err error) {
	captcha := base64Captcha.NewCaptcha(captchaDriver, captchaRedisStore{client: store.client})
	id, image, _, err = captcha.Generate()
	return
}
