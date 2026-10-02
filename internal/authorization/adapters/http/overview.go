package httpadapter

import (
	"admin/internal/authorization/application"
	"admin/internal/platform/httpresponse"
	"github.com/gin-gonic/gin"
)

type authorizationOverviewResponse struct {
	Data application.AuthorizationOverview `json:"data"`
}

type authorizationRiskPageResponse struct {
	Data application.AuthorizationRiskPage `json:"data"`
}

type authorizationRiskListQuery struct {
	Page     int    `form:"page" binding:"omitempty,min=1,max=1000000"`
	Size     int    `form:"size" binding:"omitempty,min=1,max=100"`
	Kind     string `form:"kind"`
	Resource string `form:"resource"`
	Keyword  string `form:"keyword"`
}

func (handler *Handler) getAuthorizationOverview(c *gin.Context) {
	result, err := handler.service.GetAuthorizationOverview(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		badRequest(c, err)
		return
	}
	success(c, result)
}

func (handler *Handler) listAuthorizationRisks(c *gin.Context) {
	query := authorizationRiskListQuery{Page: 1, Size: 10}
	if err := c.ShouldBindQuery(&query); err != nil {
		httpresponse.WriteError(c, authzValidation(), err, nil)
		return
	}
	result, err := handler.service.ListAuthorizationRisks(c.Request.Context(), c.GetUint("userID"), application.AuthorizationRiskListRequest{
		Page: query.Page, Size: query.Size, Kind: application.RiskKind(query.Kind), Resource: application.RiskResource(query.Resource), Keyword: query.Keyword,
	})
	if err != nil {
		badRequest(c, err)
		return
	}
	success(c, result)
}
