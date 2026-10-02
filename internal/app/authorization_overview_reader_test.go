package app

import (
	"context"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	authgorm "admin/internal/authorization/adapters/gorm"
	authapplication "admin/internal/authorization/application"
	authdomain "admin/internal/authorization/domain"
	identitydomain "admin/internal/identity/domain"
	"admin/internal/navigation"
	"admin/internal/organization"
	"admin/testsupport/testutil"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type overviewReaderUsersFake struct {
	users []identitydomain.DirectoryUser
}

func (fake overviewReaderUsersFake) ListUsersByIDs(_ context.Context, ids []uint) ([]identitydomain.DirectoryUser, error) {
	result := make([]identitydomain.DirectoryUser, 0, len(ids))
	for _, id := range ids {
		for _, user := range fake.users {
			if user.ID == id {
				result = append(result, user)
				break
			}
		}
	}
	return result, nil
}

func (fake overviewReaderUsersFake) ListUserIDs(context.Context) ([]uint, error) {
	result := make([]uint, len(fake.users))
	for index, user := range fake.users {
		result[index] = user.ID
	}
	return result, nil
}

var _ authapplication.OverviewUserDirectory = overviewReaderUsersFake{}

type countingGORMLogger struct {
	logger.Interface
	mu     sync.Mutex
	traces int
}

func (logger *countingGORMLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	logger.mu.Lock()
	logger.traces++
	logger.mu.Unlock()
	logger.Interface.Trace(ctx, begin, fc, err)
}

func (logger *countingGORMLogger) count() int {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	return logger.traces
}

func TestAuthorizationOverviewReaderUsesFixedBatchQueries(t *testing.T) {
	db := testutil.OpenIsolatedSQLite(t)
	if err := db.AutoMigrate(append(append(authgorm.Models(), navigation.Models()...), organization.Models()...)...); err != nil {
		t.Fatalf("migrate overview reader fixture: %v", err)
	}
	roles := make([]authgorm.Role, 30)
	users := make([]identitydomain.DirectoryUser, 30)
	for index := range roles {
		roles[index] = authgorm.Role{Name: "Role-" + string(rune('a'+index)), Code: "role-" + string(rune('a'+index)), Status: 1, DataScope: string(authdomain.DataScopeAll)}
		users[index] = identitydomain.DirectoryUser{ID: uint(index + 1), Username: "user", Status: 1}
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatalf("create overview roles: %v", err)
	}
	assignments := make([]authgorm.UserRole, len(roles))
	for index := range roles {
		assignments[index] = authgorm.UserRole{UserID: users[index].ID, RoleID: roles[index].ID}
	}
	if err := db.Create(&assignments).Error; err != nil {
		t.Fatalf("create overview assignments: %v", err)
	}
	counter := &countingGORMLogger{Interface: logger.New(log.New(io.Discard, "", 0), logger.Config{LogLevel: logger.Silent})}
	reader := authorizationOverviewReader{
		authorization: authgorm.NewRepository(db.Session(&gorm.Session{Logger: counter})),
		navigation:    navigation.NewGORMRepository(db.Session(&gorm.Session{Logger: counter})),
		organizations: organization.NewGORMRepository(db.Session(&gorm.Session{Logger: counter})),
		users:         overviewReaderUsersFake{users: users},
	}
	_, err := reader.ReadAuthorizationFacts(context.Background(), authapplication.AuthorizationOverviewScope{
		User: authdomain.AllUsersScope(), Organization: authdomain.AllOrganizationsScope(), DataScope: string(authdomain.DataScopeAll),
	})
	if err != nil {
		t.Fatalf("read overview facts: %v", err)
	}
	if got := counter.count(); got != 8 {
		t.Fatalf("overview reader query count = %d, want fixed batch count 8", got)
	}
}
