package dto

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

// ========== 用户自改 ==========

type UpdateSelfReq struct {
	Nickname string `json:"nickname" binding:"max=100"`
	Avatar   string `json:"avatar"   binding:"max=255"`
	Email    string `json:"email"    binding:"email"`
}

type ChangePasswordReq struct {
	OldPassword     string `json:"old_password"     binding:"required"`
	NewPassword     string `json:"new_password"     binding:"required,min=6,max=255"`
	ConfirmPassword string `json:"confirm_password" binding:"required,min=6,max=255"`
}

// ========== 管理端 ==========

type UserListResp struct {
	List  []UserInfo `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

type AdminUpdateUserReq struct {
	Nickname string `json:"nickname" binding:"max=100"`
	Email    string `json:"email"    binding:"email"`
	Role     string `json:"role"`
	Status   *int   `json:"status"`
}

// ========== 初始上下文 ==========

type InitialContextResp struct {
	User        UserInfo     `json:"user"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"permissions"`
	Menus       []MenuDetail `json:"menus"`
}
