package request

// ========== 请求参数 ==========

type RegisterReq struct {
	Username    string `json:"username"  binding:"required,min=3,max=100"`
	Password    string `json:"password"  binding:"required,min=6,max=255"`
	Email       string `json:"email"     binding:"required,email"`
	Nickname    string `json:"nickname"`
	CaptchaID   string `json:"captcha_id"   binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=6"`
}

type LoginReq struct {
	Username    string `json:"username"     binding:"required"`
	Password    string `json:"password"     binding:"required"`
	CaptchaID   string `json:"captcha_id"   binding:"required"`
	CaptchaCode string `json:"captcha_code" binding:"required,len=6"`
}

// ========== 响应 ==========

type UserInfo struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   int    `json:"status"`
}

type LoginResp struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserInfo `json:"user"`
}
