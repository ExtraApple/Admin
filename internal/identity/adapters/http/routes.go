package httpadapter

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"admin/internal/identity"
	"admin/internal/identity/application"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

type CaptchaGenerator interface {
	GenerateCaptcha() (string, string, error)
}

type captchaResponse struct {
	Data struct {
		CaptchaID    string `json:"captcha_id"`
		CaptchaImage string `json:"captcha_img"`
	} `json:"data"`
}
type userResponse struct {
	Data identity.UserInfo `json:"data"`
}
type loginResponse struct {
	Data identity.LoginResponse `json:"data"`
}
type refreshResponse struct {
	Data identity.RefreshTokenResponse `json:"data"`
}

type routeHandler struct {
	service        *application.Service
	users          *application.UserService
	context        *application.ContextService
	avatars        *application.AvatarService
	captcha        CaptchaGenerator
	avatarMaxBytes int64
	logoutExpires  time.Duration
}

func Routes(service *application.Service, users *application.UserService, contextService *application.ContextService, avatars *application.AvatarService, captcha CaptchaGenerator, avatarMaxSizeMB int, logoutExpires ...time.Duration) []routecatalog.Descriptor {
	expires := time.Duration(0)
	if len(logoutExpires) > 0 {
		expires = logoutExpires[0]
	}
	handler := &routeHandler{service: service, users: users, context: contextService, avatars: avatars, captcha: captcha, avatarMaxBytes: int64(avatarMaxSizeMB) * 1024 * 1024, logoutExpires: expires}
	return []routecatalog.Descriptor{
		identityRoute(http.MethodGet, "/api/captcha", "Generate Captcha", routecatalog.Public, handler.captchaHandler, nil, captchaResponse{}),
		identityRoute(http.MethodPost, "/api/register", "Register", routecatalog.Public, handler.register, identity.RegisterRequest{}, userResponse{}),
		identityRoute(http.MethodPost, "/api/login", "Login", routecatalog.Public, handler.login, identity.LoginRequest{}, loginResponse{}),
		identityRoute(http.MethodPost, "/api/refresh", "Refresh Token", routecatalog.Public, handler.refresh, identity.RefreshTokenRequest{}, refreshResponse{}),
		identityRoute(http.MethodGet, "/api/user/context", "Get User Context", routecatalog.Authenticated, handler.initialContext, nil, identity.InitialContextResponse{}),
		avatarContentRoute("/api/avatars/default", "Get Default Avatar", handler.getDefaultAvatar),
		avatarContentRoute("/api/avatars/:user_id", "Get User Avatar", handler.getAvatar),
		identityRoute(http.MethodGet, "/api/user/info", "Get User Info", routecatalog.Authenticated, handler.getInfo, nil, userResponse{}),
		identityRoute(http.MethodPut, "/api/user/info", "Update User Info", routecatalog.Authenticated, handler.updateSelf, identity.UpdateSelfRequest{}, userResponse{}),
		identityRoute(http.MethodPut, "/api/user/password", "Change Password", routecatalog.Authenticated, handler.changePassword, identity.ChangePasswordRequest{}, nil),
		avatarUploadRoute(handler.uploadAvatar),
		identityRoute(http.MethodDelete, "/api/user/avatar", "Restore Default Avatar", routecatalog.Authenticated, handler.restoreDefaultAvatar, nil, userResponse{}),
		identityRoute(http.MethodPost, "/api/user/logout", "Logout", routecatalog.Authenticated, handler.logout, nil, nil),
		permissionIdentityRoute(http.MethodGet, "/api/admin/users", "List Users", "admin.users.get", handler.listUsers, nil, identity.UserListResponse{}),
		permissionIdentityRoute(http.MethodPut, "/api/admin/users/:id", "Update User", "admin.users.id.put", handler.updateUser, identity.AdminUpdateUserRequest{}, userResponse{}),
		permissionIdentityRoute(http.MethodDelete, "/api/admin/users/:id", "Delete User", "admin.users.id.delete", handler.deleteUser, nil, nil),
		permissionIdentityRoute(http.MethodPut, "/api/admin/users/:id/status", "Toggle User Status", "admin.users.id.status.put", handler.toggleStatus, nil, nil),
		permissionIdentityRoute(http.MethodPut, "/api/admin/users/:id/kick", "Kick User", "admin.users.id.kick.put", handler.kickUser, nil, nil),
	}

}

func identityRoute(method, path, name string, access routecatalog.AccessLevel, handler gin.HandlerFunc, request, response any) routecatalog.Descriptor {
	requestBody := routecatalog.RequestBody{Kind: routecatalog.NoBody}
	if request != nil {
		requestBody = routecatalog.RequestBody{Kind: routecatalog.JSONBody, Schema: reflect.TypeOf(request), Required: true}
	}
	responses := map[int]routecatalog.Response{
		http.StatusOK: routecatalog.JSONResponse("success", routecatalog.DataSchemaOf(response)),
	}
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError} {
		responses[status] = identityErrorResponse(status)
	}
	for _, code := range []string{"REQUEST_INVALID", "AVATAR_FIELD_NOT_WRITABLE", "UPLOAD_BODY_INVALID", "UPLOAD_BODY_TOO_LARGE", "UPLOAD_FILE_MISSING", "UPLOAD_MULTIPLE_FILES", "FILE_EMPTY", "FILE_TOO_LARGE", "FILE_TYPE_NOT_ALLOWED", "FILE_TYPE_MISMATCH", "FILE_ENCODING_INVALID", "FILE_CONTENT_INVALID", "IMAGE_DIMENSION_LIMIT", "IMAGE_DECODE_INVALID", "STORAGE_OBJECT_NOT_FOUND", "STORAGE_UNAVAILABLE", "PERSISTENCE_FAILED", "INTERNAL_ERROR"} {
		definition := avatarDefinition(code, nil)
		response, exists := responses[definition.Status]
		if !exists {
			responses[definition.Status] = routecatalog.ErrorResponse(definition.Message, definition)
			continue
		}
		response.Errors = append(response.Errors, definition)
		responses[definition.Status] = response
	}
	return routecatalog.Descriptor{Method: method, Path: path, Access: access, Handler: handler, Name: name, Group: "identity", DefaultAuditCategory: "identity", OpenAPI: routecatalog.Operation{Summary: name, Request: requestBody, Responses: responses}}
}

func permissionIdentityRoute(method, path, name, permission string, handler gin.HandlerFunc, request, response any) routecatalog.Descriptor {
	descriptor := identityRoute(method, path, name, routecatalog.PermissionControlled, handler, request, response)
	descriptor.DefaultPermissionCode = permission
	return descriptor
}

func avatarContentRoute(path, name string, handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := identityRoute(http.MethodGet, path, name, routecatalog.Public, handler, nil, struct{}{})
	descriptor.OpenAPI.Responses[http.StatusOK] = routecatalog.Response{Description: "avatar image", Kind: routecatalog.BinaryBody, ContentTypes: []string{"image/jpeg", "image/png"}}
	return descriptor
}

func avatarUploadRoute(handler gin.HandlerFunc) routecatalog.Descriptor {
	descriptor := identityRoute(http.MethodPost, "/api/user/avatar", "Upload Avatar", routecatalog.Authenticated, handler, nil, userResponse{})
	descriptor.OpenAPI.Request = routecatalog.RequestBody{Kind: routecatalog.MultipartBody, Required: true, FileField: "file", FileDescription: "JPEG or PNG avatar"}
	return descriptor
}

func (handler *routeHandler) captchaHandler(c *gin.Context) {
	if handler.captcha == nil {
		httpresponse.WriteError(c, identityInternal(), nil, nil)
		return
	}
	id, image, err := handler.captcha.GenerateCaptcha()
	if err != nil {
		httpresponse.WriteError(c, identityInternal(), err, nil)
		return
	}
	identitySuccess(c, map[string]string{"captcha_id": id, "captcha_img": image})
}
func (handler *routeHandler) register(c *gin.Context) {
	var request identity.RegisterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		identityRequestError(c, err)
		return
	}
	user, err := handler.service.Register(c.Request.Context(), application.RegisterRequest{Username: request.Username, Password: request.Password, Email: request.Email, Nickname: request.Nickname, CaptchaID: request.CaptchaID, CaptchaCode: request.CaptchaCode})
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(user))
}

func (handler *routeHandler) login(c *gin.Context) {
	var request identity.LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		identityRequestError(c, err)
		return
	}
	result, err := handler.service.Login(c.Request.Context(), application.LoginRequest{Username: request.Username, Password: request.Password, CaptchaID: request.CaptchaID, CaptchaCode: request.CaptchaCode})
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.LoginResponse{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, User: identity.UserInfoFromDomain(result.User)})
}

func (handler *routeHandler) refresh(c *gin.Context) {
	var request identity.RefreshTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeIdentityError(c, err, true)
		return
	}
	pair, err := handler.service.Refresh(c.Request.Context(), application.RefreshRequest{RefreshToken: request.RefreshToken})
	if err != nil {
		writeIdentityError(c, err, true)
		return
	}
	identitySuccess(c, identity.RefreshTokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken})
}

func (handler *routeHandler) logout(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if err := handler.service.Logout(c.Request.Context(), token, handler.logoutExpires); err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, nil)
}
func (handler *routeHandler) getInfo(c *gin.Context) {
	user, err := handler.users.Get(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(user))
}

func (handler *routeHandler) updateSelf(c *gin.Context) {
	var request identity.UpdateSelfRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		identityRequestError(c, err)
		return
	}
	user, err := handler.users.UpdateSelf(c.Request.Context(), c.GetUint("userID"), application.UpdateSelfRequest{Nickname: request.Nickname, Email: request.Email, AvatarPresent: request.HasAvatarField()})
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(user))
}

func (handler *routeHandler) changePassword(c *gin.Context) {
	var request identity.ChangePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		identityRequestError(c, err)
		return
	}
	err := handler.users.ChangePassword(c.Request.Context(), c.GetUint("userID"), application.ChangePasswordRequest{OldPassword: request.OldPassword, NewPassword: request.NewPassword, ConfirmPassword: request.ConfirmPassword})
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, nil)
}

func (handler *routeHandler) listUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))
	result, err := handler.users.List(c.Request.Context(), c.GetUint("userID"), page, size)
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	list := make([]identity.UserInfo, len(result.List))
	for index := range result.List {
		list[index] = identity.UserInfoFromDomain(result.List[index])
	}
	identitySuccess(c, identity.UserListResponse{List: list, Total: result.Total, Page: page, Size: size})
}

func (handler *routeHandler) updateUser(c *gin.Context) {
	targetID, ok := parseTargetID(c)
	if !ok {
		return
	}
	var request identity.AdminUpdateUserRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		identityRequestError(c, err)
		return
	}
	user, err := handler.users.UpdateByAdmin(c.Request.Context(), c.GetUint("userID"), targetID, application.AdminUpdateUserRequest{Nickname: request.Nickname, Email: request.Email, Role: request.Role, Status: request.Status})
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(user))
}

func (handler *routeHandler) deleteUser(c *gin.Context) {
	targetID, ok := parseTargetID(c)
	if !ok {
		return
	}
	if err := handler.users.Delete(c.Request.Context(), c.GetUint("userID"), targetID); err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, nil)
}

func (handler *routeHandler) toggleStatus(c *gin.Context) {
	targetID, ok := parseTargetID(c)
	if !ok {
		return
	}
	status, err := handler.users.ToggleStatus(c.Request.Context(), c.GetUint("userID"), targetID)
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, map[string]int{"status": status})
}

func (handler *routeHandler) kickUser(c *gin.Context) {
	targetID, ok := parseTargetID(c)
	if !ok {
		return
	}
	if err := handler.users.Kick(c.Request.Context(), c.GetUint("userID"), targetID); err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, nil)
}

func parseTargetID(c *gin.Context) (uint, bool) {
	targetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || targetID == 0 {
		identityRequestError(c, err)
		return 0, false
	}
	return uint(targetID), true
}

func badRequest(c *gin.Context, err error) {
	identityRequestError(c, err)
}

func (handler *routeHandler) initialContext(c *gin.Context) {
	result, err := handler.context.Load(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		writeIdentityError(c, err, false)
		return
	}
	identitySuccess(c, identity.InitialContextResponse{User: identity.UserInfoFromDomain(result.User), Roles: result.Roles, Permissions: result.Permissions, Menus: identity.MenuDetailsFromDomain(result.Menus)})
}

const avatarMultipartOverheadBytes = int64(256 * 1024)
const uploadMultipartMemoryBytes = int64(1 * 1024 * 1024)

func (handler *routeHandler) uploadAvatar(c *gin.Context) {
	file, cleanup, err := parseAvatarUpload(c, handler.avatarMaxBytes)
	defer cleanup()
	if err != nil {
		writeAvatarError(c, err)
		return
	}
	reader, err := file.Open()
	if err != nil {
		writeAvatarError(c, application.NewAvatarError("UPLOAD_BODY_INVALID", err))
		return
	}
	defer reader.Close()
	if handler.avatars == nil {
		writeAvatarError(c, application.NewAvatarError(application.AvatarCodeInternalError, nil))
		return
	}
	result, err := handler.avatars.Upload(c.Request.Context(), application.UploadAvatarInput{UserID: c.GetUint("userID"), FileName: file.Filename, ContentType: file.Header.Get("Content-Type"), Size: file.Size, MaxBytes: handler.avatarMaxBytes, Reader: reader})
	if err != nil {
		writeAvatarError(c, err)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(result.User))
}

func (handler *routeHandler) restoreDefaultAvatar(c *gin.Context) {
	if handler.avatars == nil {
		writeAvatarError(c, application.NewAvatarError(application.AvatarCodeInternalError, nil))
		return
	}
	user, err := handler.avatars.RestoreDefault(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		writeAvatarError(c, err)
		return
	}
	identitySuccess(c, identity.UserInfoFromDomain(user))
}

func (handler *routeHandler) getAvatar(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || userID == 0 || handler.avatars == nil {
		writeAvatarContent(c, application.DefaultAvatarContent(), "public, max-age=300")
		return
	}
	writeAvatarContent(c, handler.avatars.Open(c.Request.Context(), uint(userID)), "public, max-age=300")
}

func (handler *routeHandler) getDefaultAvatar(c *gin.Context) {
	writeAvatarContent(c, application.DefaultAvatarContent(), "public, max-age=86400")
}

func writeAvatarContent(c *gin.Context, content application.AvatarContent, cacheControl string) {
	if content.Reader == nil || (content.ContentType != "image/jpeg" && content.ContentType != "image/png") {
		if content.Reader != nil {
			_ = content.Reader.Close()
		}
		content = application.DefaultAvatarContent()
	}
	defer content.Reader.Close()
	c.Header("Content-Type", content.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", cacheControl)
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, content.Reader); err != nil {
		_ = c.Error(err)
	}
}

func parseAvatarUpload(c *gin.Context, maxFileBytes int64) (*multipart.FileHeader, func(), error) {
	if maxFileBytes <= 0 {
		maxFileBytes = 5 * 1024 * 1024
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFileBytes+avatarMultipartOverheadBytes)
	if err := c.Request.ParseMultipartForm(uploadMultipartMemoryBytes); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return nil, func() {}, application.NewAvatarError("UPLOAD_BODY_TOO_LARGE", err)
		}
		return nil, func() {}, application.NewAvatarError("UPLOAD_BODY_INVALID", err)
	}
	form := c.Request.MultipartForm
	cleanup := func() {
		if form != nil {
			_ = form.RemoveAll()
		}
	}
	total := 0
	for _, files := range form.File {
		total += len(files)
	}
	if total == 0 {
		return nil, cleanup, application.NewAvatarError("UPLOAD_FILE_MISSING", nil)
	}
	files := form.File["file"]
	if total != 1 || len(files) != 1 {
		return nil, cleanup, application.NewAvatarError("UPLOAD_MULTIPLE_FILES", nil)
	}
	if files[0].Size == 0 {
		return nil, cleanup, application.NewAvatarError("FILE_EMPTY", nil)
	}
	if files[0].Size > maxFileBytes {
		return nil, cleanup, application.NewAvatarError("FILE_TOO_LARGE", nil)
	}
	return files[0], cleanup, nil
}

func writeAvatarError(c *gin.Context, err error) {
	code, ok := application.AvatarErrorCode(err)
	if !ok {
		code = application.AvatarCodeInternalError
	}
	definition := avatarDefinition(code, err)
	httpresponse.WriteError(c, definition, err, nil)
}
