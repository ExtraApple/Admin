package service

import (
	"errors"

	"admin/global"
	"admin/model"
	"admin/request"
)

// ========== 管理端 ==========

// --- 获取用户列表 ---
func GetAllUsers(page, pageSize int) ([]request.UserInfo, int64, error) {
	var users []model.User
	var total int64

	global.DB.Model(&model.User{}).Count(&total)
	if err := global.DB.Limit(pageSize).Offset((page - 1) * pageSize).Find(&users).Error; err != nil {
		return nil, 0, errors.New("查询用户列表失败")
	}

	list := make([]request.UserInfo, len(users))
	for i, u := range users {
		list[i] = request.UserInfo{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname,
			Avatar: u.Avatar, Email: u.Email, Role: u.Role, Status: u.Status,
		}
	}
	return list, total, nil
}

// --- 删除用户 ---
func DeleteUserByAdmin(operatorID, targetID uint) error {
	if operatorID == targetID {
		return errors.New("不能删除自己")
	}
	var user model.User
	if err := global.DB.First(&user, targetID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if user.Role == "admin" {
		return errors.New("不能删除其他管理员")
	}
	return global.DB.Delete(&user).Error
}

// --- 管理员修改用户 ---
func UpdateUserByAdmin(operatorID, targetID uint, req request.AdminUpdateUserReq) (*request.UserInfo, error) {
	if operatorID == targetID {
		return nil, errors.New("不能修改自己的信息（请使用普通用户修改接口）")
	}
	var target model.User
	if err := global.DB.First(&target, targetID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	if target.Role == "admin" {
		return nil, errors.New("不能修改其他管理员")
	}

	updates := map[string]interface{}{}
	if req.Nickname != "" {
		updates["nickname"] = req.Nickname
	}
	if req.Email != "" {
		var exist int64
		global.DB.Model(&model.User{}).Where("email = ? AND id != ?", req.Email, targetID).Count(&exist)
		if exist > 0 {
			return nil, errors.New("邮箱已被占用")
		}
		updates["email"] = req.Email
	}
	if req.Role != "" {
		updates["role"] = req.Role
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if len(updates) == 0 {
		return nil, errors.New("无修改内容")
	}
	if err := global.DB.Model(&target).Updates(updates).Error; err != nil {
		return nil, errors.New("修改失败")
	}
	// 刷新返回最新数据
	global.DB.First(&target, targetID)
	return &request.UserInfo{
		ID: target.ID, Username: target.Username, Nickname: target.Nickname,
		Avatar: target.Avatar, Email: target.Email, Role: target.Role, Status: target.Status,
	}, nil
}

// --- 切换用户状态 ---
func ToggleUserStatus(operatorID, targetID uint) (int, error) {
	if operatorID == targetID {
		return 0, errors.New("不能操作自己")
	}
	var user model.User
	if err := global.DB.First(&user, targetID).Error; err != nil {
		return 0, errors.New("用户不存在")
	}
	if user.Role == "admin" {
		return 0, errors.New("不能操作其他管理员")
	}
	newStatus := 1
	if user.Status == 1 {
		newStatus = 0
	}
	if err := global.DB.Model(&user).Update("status", newStatus).Error; err != nil {
		return 0, errors.New("操作失败")
	}
	return newStatus, nil
}
