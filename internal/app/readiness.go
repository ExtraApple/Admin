package app

import (
	"context"
	"net/http"
	"reflect"
	"sync"

	messagingapplication "admin/internal/messaging/application"
	messagingdomain "admin/internal/messaging/domain"
	"admin/internal/platform/httpresponse"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
)

func readyDescriptor(readiness *messagingReadiness) routecatalog.Descriptor {
	return routecatalog.Descriptor{Method: http.MethodGet, Path: "/api/ready", Access: routecatalog.Public, Handler: func(c *gin.Context) {
		httpresponse.WriteSuccess(c, http.StatusOK, readiness.Snapshot(c.Request.Context()))
	}, Name: "Readiness", Group: "system", DefaultAuditCategory: "readiness", OpenAPI: routecatalog.Operation{
		Summary: "Check readiness", Description: "Reports safe application and RabbitMQ readiness state.", Request: routecatalog.RequestBody{Kind: routecatalog.NoBody}, Responses: map[int]routecatalog.Response{http.StatusOK: routecatalog.JSONResponse("ready", reflect.TypeOf(ReadyResponse{}))},
	}}
}

type messagingBrokerState struct {
	mu            sync.RWMutex
	healthy       bool
	lastErrorCode string
}

func (state *messagingBrokerState) Healthy() bool {
	if state == nil {
		return false
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.healthy
}

func (state *messagingBrokerState) LastErrorCode() string {
	if state == nil {
		return ""
	}
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.lastErrorCode
}

func (state *messagingBrokerState) MarkHealthy() {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.healthy = true
	state.lastErrorCode = ""
	state.mu.Unlock()
}

func (state *messagingBrokerState) MarkUnavailable(code string) {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.healthy = false
	state.lastErrorCode = code
	state.mu.Unlock()
}

type messagingBrokerReadiness interface {
	Healthy() bool
	LastErrorCode() string
}

type messagingOutboxCounter interface {
	ListOutboxes(context.Context, messagingapplication.OutboxListQuery) ([]messagingdomain.MessageOutbox, int64, error)
}

type messagingReadiness struct {
	configured bool
	broker     messagingBrokerReadiness
	outboxes   messagingOutboxCounter
}

type ReadyResponse struct {
	Status     string                  `json:"status"`
	Components ReadyResponseComponents `json:"components"`
}

type ReadyResponseComponents struct {
	RabbitMQ ReadyRabbitMQComponent `json:"rabbitmq"`
}

type ReadyRabbitMQComponent struct {
	Status        string `json:"status"`
	OutboxPending int64  `json:"outbox_pending"`
	LastErrorCode string `json:"last_error_code"`
}

func newMessagingReadiness(configured bool, broker messagingBrokerReadiness, outboxes messagingOutboxCounter) *messagingReadiness {
	return &messagingReadiness{configured: configured, broker: broker, outboxes: outboxes}
}

func (readiness *messagingReadiness) Snapshot(ctx context.Context) ReadyResponse {
	response := ReadyResponse{Status: "ready", Components: ReadyResponseComponents{RabbitMQ: ReadyRabbitMQComponent{Status: "disabled"}}}
	if readiness == nil || !readiness.configured {
		return response
	}
	response.Status = "degraded"
	response.Components.RabbitMQ.Status = "degraded"
	if readiness.broker != nil && readiness.broker.Healthy() {
		response.Status = "ready"
		response.Components.RabbitMQ.Status = "ready"
	}
	if readiness.broker != nil {
		response.Components.RabbitMQ.LastErrorCode = readiness.broker.LastErrorCode()
	}
	if readiness.outboxes != nil {
		_, pending, err := readiness.outboxes.ListOutboxes(ctx, messagingapplication.OutboxListQuery{Statuses: []messagingdomain.OutboxStatus{messagingdomain.OutboxStatusPending, messagingdomain.OutboxStatusPublishing}})
		if err == nil {
			response.Components.RabbitMQ.OutboxPending = pending
		}
	}
	return response
}
