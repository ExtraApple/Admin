package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"admin/service"
)

// CaptchaHandler 验证码 HTTP 处理器。
//
// 由于验证码不依赖用户认证状态，该 Handler 零依赖注入，
// 直接声明空结构体即可在工作。
type CaptchaHandler struct{}

// Generate 获取验证码图片。
//
// 路由:  GET /api/captcha
// 鉴权:  无需认证（公开接口）
// 请求体: 无
//
// 成功响应 (200):
//
//	{
//	  "code": 200,
//	  "data": {
//	    "captcha_id":  "a1b2c3d4...",   // 唯一标识，注册/登录时需回传
//	    "captcha_img": "data:image/png;base64,..." // 可直接放入 <img src>
//	  }
//	}
//
// 失败响应 (500):
//
//	{ "code": 500, "msg": "验证码生成失败" }
func (h *CaptchaHandler) Generate(c *gin.Context) {
	id, b64s, err := service.GenerateCaptcha()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "验证码生成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"captcha_id":  id,   // 唯一标识，前端需保存并在注册/登录时回传
			"captcha_img": b64s, // base64 图片，格式: data:image/png;base64,...
		},
	})
}
