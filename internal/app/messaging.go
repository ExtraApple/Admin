package app

import (
	"context"
	"strconv"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	filesapplication "admin/internal/files/application"
	identityapplication "admin/internal/identity/application"
	messaginggorm "admin/internal/messaging/adapters/gorm"
	messagingrabbitmq "admin/internal/messaging/adapters/rabbitmq"
	messagingredis "admin/internal/messaging/adapters/redis"
	messagingapplication "admin/internal/messaging/application"
	messagingdomain "admin/internal/messaging/domain"
	"admin/internal/organization"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	platformrabbitmq "admin/internal/platform/rabbitmq"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const messagingConsumerName = "websocket"

type messagingComposition struct {
	service   *messagingapplication.Service
	tickets   *messagingapplication.WebSocketTicketService
	gateway   *messagingapplication.WebSocketGateway
	refresh   *messagingapplication.RefreshHub
	readiness *messagingReadiness
	jobs      []BackgroundJob
	close     func() error
}

func newMessagingComposition(resources Resources, config platformconfig.Config, identity identityCore, authorization *authapplication.Service, files filesComposition) messagingComposition {
	repository := messaginggorm.NewRepository(resources.DB)
	configured := messagingBrokerConfigured(config.RabbitMQ)
	brokerState := &messagingBrokerState{}
	organizations := organization.NewGORMRepository(resources.DB)
	scope := organization.NewScopeReader(organizations, organization.NewHierarchy(organizations))
	roles := authgorm.NewRepository(resources.DB)
	identityReader := messagingIdentityReader{users: identity.repository}
	organizationReader := messagingOrganizationReader{scope: scope, roles: roles}
	var retryDelays []time.Duration
	var factory *messagingrabbitmq.ResilientChannelFactory
	workerID := "messaging-" + uuid.NewString()
	runtimeLogger := messagingRuntimeLogger{logger: resources.Logger}
	var replayService *messagingapplication.ConsumerDeadLetterReplayService
	if configured {
		retryDelays = messagingRetryDelays(config.RabbitMQ)
		factory = messagingrabbitmq.NewResilientChannelFactory(func(ctx context.Context) (*amqp.Connection, error) {
			return platformrabbitmq.OpenContext(ctx, config.RabbitMQ)
		}, messagingrabbitmq.Topology{Consumer: messagingConsumerName, RetryDelays: retryDelays, DLQRetention: messagingDLQRetention(config.RabbitMQ)})
		replayService = messagingapplication.NewConsumerDeadLetterReplayService(messagingapplication.ConsumerDeadLetterReplayConfig{Store: repository, Publisher: messagingrabbitmq.NewConsumerReplayPublisher(factory), WorkerID: workerID, Logger: runtimeLogger})
	}
	service := messagingapplication.NewService(messagingapplication.Dependencies{
		Messages: repository, Categories: repository, Inbox: repository, Notifications: repository,
		Outboxes: repository, ConsumerDeadLetters: repository, ConsumerDeadLetterReplay: replayService,
		Identity: identityReader, Organizations: organizationReader,
		Authorization: messagingAuthorizationReader{service: authorization}, Files: messagingFilesReader{service: files.service},
		Transactions: platformdatabase.NewTransactionRunner(resources.DB),
		ContentLimits: messagingdomain.ContentLimits{
			MaxTitleRunes: config.Messaging.MaxTitleRunes,
			MaxBodyRunes:  config.Messaging.MaxBodyRunes,
		},
	})
	hub := messagingapplication.NewRefreshHub()
	readiness := newMessagingReadiness(configured, brokerState, repository)

	ticketService := messagingapplication.NewWebSocketTicketService(messagingredis.NewWebSocketTicketStore(resources.Redis))
	gateway := messagingapplication.NewWebSocketGateway(messagingapplication.WebSocketGatewayConfig{
		Tickets:  ticketService,
		Hub:      hub,
		Recovery: messagingredis.NewRefreshReplayStore(resources.Redis),
	})
	composition := messagingComposition{
		service: service, tickets: ticketService, gateway: gateway, refresh: hub, readiness: readiness,
		jobs: []BackgroundJob{{Name: "messaging-refresh-subscriber", Run: func(ctx context.Context) {
			runMessagingRefreshSubscriber(ctx, messagingredis.NewRefreshSubscriber(resources.Redis), hub, runtimeLogger)
		}}},
	}
	if !configured {
		return composition
	}
	retryDelays = messagingRetryDelays(config.RabbitMQ)
	outbox := messagingapplication.NewOutboxWorker(messagingapplication.OutboxWorkerConfig{
		Store:          repository,
		Publisher:      messagingrabbitmq.NewPublisher(factory),
		WorkerID:       workerID,
		Lease:          time.Duration(config.RabbitMQ.WorkerLeaseSeconds) * time.Second,
		ConfirmTimeout: time.Duration(config.RabbitMQ.ConfirmTimeoutSeconds) * time.Second,
		RetryDelays:    retryDelays,
		Logger:         runtimeLogger,
	})
	recorder := messagingapplication.NewConsumerDeadLetterRecorder(messagingapplication.ConsumerDeadLetterRecorderConfig{Store: repository, Metrics: messagingZapMetrics{logger: runtimeLogger}, Logger: runtimeLogger})
	failureRouter := messagingrabbitmq.NewRecordingFailureRouter(messagingrabbitmq.NewConsumerFailurePublisher(messagingConsumerName, factory))
	consumer := messagingapplication.NewEventConsumer(messagingapplication.EventConsumerConfig{
		ConsumerName:         messagingConsumerName,
		WorkerID:             workerID,
		Lease:                time.Duration(config.RabbitMQ.WorkerLeaseSeconds) * time.Second,
		BatchSize:            messagingapplication.DefaultConsumerBatchSize,
		MaxAudienceUsers:     config.Messaging.MaxAudienceUsers,
		Messages:             repository,
		Organizations:        organizationReader,
		Identity:             identityReader,
		Store:                repository,
		Stream:               messagingredis.NewRefreshStreamPublisher(resources.Redis),
		SnapshotTransactions: platformdatabase.NewTransactionRunner(resources.DB),
		Logger:               runtimeLogger,
	})
	competing := messagingrabbitmq.NewCompetingConsumer(messagingrabbitmq.CompetingConsumerConfig{Queue: messagingrabbitmq.ConsumerQueueName(messagingConsumerName), ConsumerName: messagingConsumerName, Channels: factory, Processor: consumer, Failures: failureRouter, Logger: runtimeLogger})
	composition.jobs = append(composition.jobs,
		BackgroundJob{Name: "messaging-outbox", Run: func(ctx context.Context) { runMessagingOutbox(ctx, outbox, runtimeLogger, brokerState) }},
		BackgroundJob{Name: "messaging-consumer", Run: func(ctx context.Context) { runMessagingConsumer(ctx, competing, runtimeLogger, brokerState) }},
		BackgroundJob{Name: "messaging-cleanup", Run: func(ctx context.Context) { runMessagingCleanup(ctx, repository, runtimeLogger) }},
	)
	for attempt := 1; attempt <= len(retryDelays); attempt++ {
		recorderConsumer := messagingrabbitmq.NewConsumerDeadLetterConsumer(messagingrabbitmq.ConsumerDeadLetterConsumerConfig{
			Queue:         messagingrabbitmq.ConsumerDeadLetterAttemptQueueName(messagingConsumerName, attempt),
			ConsumerName:  messagingConsumerName,
			OriginalQueue: messagingrabbitmq.ConsumerQueueName(messagingConsumerName),
			Channels:      factory,
			Recorder:      recorder,
			Logger:        runtimeLogger,
		})
		currentRecorderConsumer := recorderConsumer
		composition.jobs = append(composition.jobs, BackgroundJob{Name: "messaging-dlq-recorder-" + strconv.Itoa(attempt), Run: func(ctx context.Context) {
			runMessagingDeadLetterConsumer(ctx, currentRecorderConsumer, runtimeLogger)
		}})
	}

	composition.close = factory.Close
	return composition
}

func messagingBrokerConfigured(config platformconfig.RabbitMQConfig) bool {
	return config.Host != "" && config.Port > 0
}

func messagingRetryDelays(config platformconfig.RabbitMQConfig) []time.Duration {
	seconds := config.RetryDelaysSeconds
	if len(seconds) != 5 {
		seconds = []int{1, 2, 4, 8, 16}
	}
	delays := make([]time.Duration, len(seconds))
	for index, second := range seconds {
		delays[index] = time.Duration(second) * time.Second
	}
	return delays
}

func messagingDLQRetention(config platformconfig.RabbitMQConfig) time.Duration {
	days := config.DLQRetentionDays
	if days < 1 {
		days = 7
	}
	return time.Duration(days) * 24 * time.Hour
}

func runMessagingOutbox(ctx context.Context, worker *messagingapplication.OutboxWorker, logger messagingapplication.RuntimeLogger, brokerState *messagingBrokerState) {
	for {
		if _, err := worker.RunOnce(ctx, 100); err != nil && ctx.Err() == nil {
			brokerState.MarkUnavailable("rabbitmq_publish_failed")
			logger.Warn("messaging_outbox_run_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "run"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "rabbitmq_publish_failed"})
		} else if err == nil {
			brokerState.MarkHealthy()
		}
		if !waitMessagingInterval(ctx, time.Second) {
			return
		}
	}
}

func runMessagingConsumer(ctx context.Context, consumer *messagingrabbitmq.CompetingConsumer, logger messagingapplication.RuntimeLogger, brokerState *messagingBrokerState) {
	for {
		if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
			brokerState.MarkUnavailable("consumer_unavailable")
			logger.Warn("messaging_consumer_run_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "run"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "consumer_unavailable"})
		} else if err == nil {
			brokerState.MarkHealthy()
		}
		if !waitMessagingInterval(ctx, time.Second) {
			return
		}
	}
}

func runMessagingDeadLetterConsumer(ctx context.Context, consumer *messagingrabbitmq.ConsumerDeadLetterConsumer, logger messagingapplication.RuntimeLogger) {
	for {
		if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("messaging_dlq_recorder_run_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "run"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "consumer_dlq_recorder_unavailable"})
		}
		if !waitMessagingInterval(ctx, time.Second) {
			return
		}
	}
}

func runMessagingRefreshSubscriber(ctx context.Context, subscriber *messagingredis.RefreshSubscriber, hub *messagingapplication.RefreshHub, logger messagingapplication.RuntimeLogger) {
	for {
		if err := subscriber.Run(ctx, hub); err != nil && ctx.Err() == nil {
			logger.Warn("messaging_refresh_subscription_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "run"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "refresh_subscription_unavailable"})
		}
		if !waitMessagingInterval(ctx, time.Second) {
			return
		}
	}
}

func runMessagingCleanup(ctx context.Context, repository *messaginggorm.Repository, logger messagingapplication.RuntimeLogger) {
	cleanup := func() {
		now := time.Now().UTC()
		if _, err := repository.CleanupAudienceDeliveries(ctx, now, 500); err != nil && ctx.Err() == nil {
			logger.Warn("messaging_snapshot_cleanup_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "cleanup"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "snapshot_cleanup_failed"})
		}
		if _, err := repository.CleanupFinalConsumerDeadLetters(ctx, now.Add(-30*24*time.Hour), 500); err != nil && ctx.Err() == nil {
			logger.Warn("messaging_dead_letter_cleanup_deferred", messagingapplication.RuntimeLogField{Key: "stage", Value: "cleanup"}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: "dead_letter_cleanup_failed"})
		}
	}
	cleanup()
	for waitMessagingInterval(ctx, time.Hour) {
		cleanup()
	}
}

func waitMessagingInterval(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type messagingRuntimeLogger struct{ logger *zap.Logger }

func (runtime messagingRuntimeLogger) Info(message string, fields ...messagingapplication.RuntimeLogField) {
	runtime.log(zap.InfoLevel, message, fields...)
}

func (runtime messagingRuntimeLogger) Warn(message string, fields ...messagingapplication.RuntimeLogField) {
	runtime.log(zap.WarnLevel, message, fields...)
}

func (runtime messagingRuntimeLogger) log(level zapcore.Level, message string, fields ...messagingapplication.RuntimeLogField) {
	if runtime.logger == nil {
		return
	}
	encoded := make([]zap.Field, 0, len(fields))
	for _, field := range fields {
		if !messagingRuntimeLogKeyAllowed(field.Key) {
			continue
		}
		switch value := field.Value.(type) {
		case string:
			encoded = append(encoded, zap.String(field.Key, value))
		case bool:
			encoded = append(encoded, zap.Bool(field.Key, value))
		case int:
			encoded = append(encoded, zap.Int(field.Key, value))
		case int64:
			encoded = append(encoded, zap.Int64(field.Key, value))
		case uint:
			encoded = append(encoded, zap.Uint(field.Key, value))
		case uint64:
			encoded = append(encoded, zap.Uint64(field.Key, value))
		case time.Duration:
			encoded = append(encoded, zap.Duration(field.Key, value))
		}
	}
	runtime.logger.Check(level, message).Write(encoded...)
}

func messagingRuntimeLogKeyAllowed(key string) bool {
	switch key {
	case "consumer", "event_id", "failure_code", "stage", "retry_attempt", "worker_id", "outbox_id", "dead_lettered", "state", "audience_observed_count", "projection_id", "replay_cycle", "result", "pending_count", "oldest_pending_age":
		return true
	default:
		return false
	}
}

type messagingZapMetrics struct {
	logger messagingapplication.RuntimeLogger
}

func (metrics messagingZapMetrics) RecordConsumerDLQPending(_ context.Context, observation messagingapplication.ConsumerDLQPendingObservation) {
	if metrics.logger == nil {
		return
	}
	metrics.logger.Warn("messaging_consumer_dlq_pending", messagingapplication.RuntimeLogField{Key: "consumer", Value: observation.ConsumerName}, messagingapplication.RuntimeLogField{Key: "failure_code", Value: observation.FailureCode}, messagingapplication.RuntimeLogField{Key: "pending_count", Value: observation.PendingCount}, messagingapplication.RuntimeLogField{Key: "oldest_pending_age", Value: observation.OldestPendingAge})
}

type messagingIdentityReader struct {
	users identityapplication.UserRepository
}

func (reader messagingIdentityReader) LookupUser(ctx context.Context, userID uint) (messagingapplication.IdentityUser, error) {
	user, err := reader.users.FindByID(ctx, userID)
	if err != nil {
		return messagingapplication.IdentityUser{}, err
	}
	displayName := user.Nickname
	if displayName == "" {
		displayName = user.Username
	}
	return messagingapplication.IdentityUser{ID: user.ID, DisplayName: displayName, Enabled: user.Status == 1}, nil
}

func (reader messagingIdentityReader) LookupVerifiedEmail(ctx context.Context, userID uint) (messagingapplication.VerifiedEmail, bool, error) {
	user, err := reader.users.FindByID(ctx, userID)
	if err != nil {
		return messagingapplication.VerifiedEmail{}, false, err
	}
	if user.Email == "" || user.EmailVerifiedAt == nil {
		return messagingapplication.VerifiedEmail{}, false, nil
	}
	return messagingapplication.VerifiedEmail{Address: user.Email}, true, nil
}

type roleMemberReader interface {
	UserIDsByRole(context.Context, uint) ([]uint, error)
}

type userRoleReader interface {
	RoleIDsByUser(context.Context, uint) ([]uint, error)
}

type messagingOrganizationReader struct {
	scope organization.ScopeReader
	roles roleMemberReader
}

func (reader messagingOrganizationReader) Memberships(ctx context.Context, userID uint) ([]messagingapplication.OrganizationMembership, error) {
	memberships, err := reader.scope.Memberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]messagingapplication.OrganizationMembership, len(memberships))
	for index, membership := range memberships {
		result[index] = messagingapplication.OrganizationMembership{OrganizationID: membership.OrganizationID, JoinedAt: membership.JoinedAt}
	}
	return result, nil
}

func (reader messagingOrganizationReader) DescendantOrganizationIDs(ctx context.Context, organizationIDs []uint) ([]uint, error) {
	return reader.scope.DescendantOrganizationIDs(ctx, organizationIDs)
}

func (reader messagingOrganizationReader) MemberUserIDs(ctx context.Context, organizationID uint) ([]uint, error) {
	return reader.scope.MemberUserIDs(ctx, organizationID)
}

func (reader messagingOrganizationReader) RoleMemberUserIDs(ctx context.Context, organizationID, roleID uint) ([]uint, error) {
	organizationUsers, err := reader.scope.MemberUserIDs(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	roleUsers, err := reader.roles.UserIDsByRole(ctx, roleID)
	if err != nil {
		return nil, err
	}
	members := make(map[uint]struct{}, len(organizationUsers))
	for _, userID := range organizationUsers {
		members[userID] = struct{}{}
	}
	result := make([]uint, 0, len(roleUsers))
	for _, userID := range roleUsers {
		if _, ok := members[userID]; ok {
			result = append(result, userID)
		}
	}
	return result, nil
}

func (reader messagingOrganizationReader) RoleIDs(ctx context.Context, userID uint) ([]uint, error) {
	roleReader, ok := reader.roles.(userRoleReader)
	if !ok {
		return nil, nil
	}
	return roleReader.RoleIDsByUser(ctx, userID)
}

type messagingAuthorizationReader struct{ service *authapplication.Service }

func (reader messagingAuthorizationReader) HasPermission(ctx context.Context, userID uint, permissionCode string) (bool, error) {
	permissions, err := reader.service.Permissions(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, code := range permissions {
		if code == permissionCode {
			return true, nil
		}
	}
	return false, nil
}

func (reader messagingAuthorizationReader) OrganizationScope(ctx context.Context, userID uint) (messagingapplication.MessageOrganizationScope, error) {
	scope, err := reader.service.ResolveOrganizationScope(ctx, authdomain.Principal{UserID: userID})
	if err != nil {
		return messagingapplication.MessageOrganizationScope{}, err
	}
	return messagingapplication.MessageOrganizationScope{All: scope.All, OrganizationIDs: scope.OrganizationIDs}, nil
}

type messagingFilesReader struct{ service *filesapplication.Service }

func (reader messagingFilesReader) UploadMessageImage(ctx context.Context, input messagingapplication.MessageImageUpload) (messagingapplication.TemporaryMessageImage, error) {
	image, err := reader.service.UploadMessageImage(ctx, filesapplication.MessageImageUploadInput{UploaderID: input.UploaderID, FileName: input.FileName, ContentType: input.ContentType, Size: input.Size, Reader: input.Reader})
	return messagingapplication.TemporaryMessageImage{ID: image.ID, ExpiresAt: image.ExpiresAt}, err
}

func (reader messagingFilesReader) BindMessageImages(ctx context.Context, input messagingapplication.MessageImageBinding) error {
	return reader.service.BindMessageImages(ctx, filesapplication.MessageImageBindRequest{ActorID: input.ActorID, MessageLogicalID: input.MessageLogicalID, ImageIDs: input.ImageIDs})
}

func (reader messagingFilesReader) OpenMessageImage(ctx context.Context, input messagingapplication.VisibleMessageImage) (messagingapplication.MessageImageContent, error) {
	image, err := reader.service.OpenMessageImage(ctx, filesapplication.MessageImageOpenRequest{ID: input.ID, MessageLogicalID: input.MessageLogicalID})
	return messagingapplication.MessageImageContent{Reader: image.Reader, ContentType: image.ContentType, Size: image.Size}, err
}

var _ messagingapplication.IdentityReader = messagingIdentityReader{}
var _ messagingapplication.OrganizationAudienceReader = messagingOrganizationReader{}
var _ messagingapplication.AuthorizationReader = messagingAuthorizationReader{}
var _ messagingapplication.MessageImageFiles = messagingFilesReader{}
