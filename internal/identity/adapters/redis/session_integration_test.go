//go:build redis_integration

package redisadapter_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	redisadapter "admin/internal/identity/adapters/redis"
	"admin/internal/identity/application"
	"github.com/redis/go-redis/v9"
)

// Every state transition below crosses the public SessionStore port. Each caller
// has its own real Redis connection pool; no fake, clock override, key mutation,
// or package-private script invocation stands in for a concurrent caller.
func TestRedisIntegrationSessionStore(t *testing.T) {
	t.Run("same_attempt_has_one_committed_candidate", testSessionConcurrentAttempt)
	t.Run("recovery_deadline_does_not_slide", testSessionRecoveryDeadline)
	t.Run("session_and_material_expiry_do_not_slide", testSessionExpiry)
	t.Run("expired_predecessor_only_recovers_committed_result", testSessionExpiredPredecessor)
	t.Run("superseded_and_abnormal_reuse_are_distinct", testSessionSupersededAndReuse)
	t.Run("logout_and_rotation_cannot_resurrect_session", testSessionLogoutInterleavings)
	t.Run("missing_and_real_redis_query_failure_are_distinct", testSessionRedisFailure)
}

type sessionRedisFixture struct {
	t       *testing.T
	ctx     context.Context
	options redis.Options
	admin   *redis.Client
}

func newSessionRedisFixture(t *testing.T) *sessionRedisFixture {
	t.Helper()
	address := os.Getenv("ADMIN_TEST_REDIS_ADDR")
	if address == "" {
		t.Fatal("ADMIN_TEST_REDIS_ADDR is required for the mandatory Redis gate")
	}
	database := 15
	if configured := os.Getenv("ADMIN_TEST_REDIS_DB"); configured != "" {
		parsed, err := strconv.Atoi(configured)
		if err != nil || parsed < 0 {
			t.Fatal("ADMIN_TEST_REDIS_DB must be a non-negative database number")
		}
		database = parsed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	fixture := &sessionRedisFixture{
		t:   t,
		ctx: ctx,
		options: redis.Options{
			Addr: address, Password: os.Getenv("ADMIN_TEST_REDIS_PASSWORD"), DB: database,
			DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second,
			MaxRetries: -1,
		},
	}
	fixture.admin = fixture.client(fixture.options)
	return fixture
}

func (fixture *sessionRedisFixture) client(options redis.Options) *redis.Client {
	fixture.t.Helper()
	client := redis.NewClient(&options)
	fixture.t.Cleanup(func() {
		if err := client.Close(); err != nil {
			fixture.t.Error("close session integration Redis client failed")
		}
	})
	if err := client.Ping(fixture.ctx).Err(); err != nil {
		fixture.t.Fatal("ping session integration Redis failed")
	}
	return client
}

func (fixture *sessionRedisFixture) caller() application.SessionStore {
	fixture.t.Helper()
	return redisadapter.NewStore(fixture.client(fixture.options))
}

func sessionTestID(t *testing.T) string {
	t.Helper()
	var entropy [24]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal("generate isolated session integration identifier failed")
	}
	return hex.EncodeToString(entropy[:])
}

func sessionTestIssue(t *testing.T, lifetime time.Duration) application.TokenIssue {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	return application.TokenIssue{
		UserID: 42, TokenVersion: 7, SessionID: sessionTestID(t), Generation: 1,
		RefreshID: sessionTestID(t), Roles: []string{"viewer"}, Permissions: []string{"profile.read"},
		IssuedAt: now, AccessExpiresAt: now.Add(time.Minute), RefreshExpiresAt: now.Add(lifetime),
	}
}

func sessionTestClaims(issue application.TokenIssue) application.TokenClaims {
	return application.TokenClaims{
		UserID: issue.UserID, TokenVersion: issue.TokenVersion, Purpose: application.TokenPurposeRefresh,
		SessionID: issue.SessionID, Generation: issue.Generation, RefreshID: issue.RefreshID,
		ExpiresAt: issue.RefreshExpiresAt, Expired: !issue.RefreshExpiresAt.After(time.Now()),
	}
}

func sessionTestSuccessor(t *testing.T, credential application.TokenClaims, lifetime time.Duration) application.TokenIssue {
	t.Helper()
	issue := sessionTestIssue(t, lifetime)
	issue.UserID, issue.TokenVersion = credential.UserID, credential.TokenVersion
	issue.SessionID, issue.Generation = credential.SessionID, credential.Generation+1
	return issue
}

func (fixture *sessionRedisFixture) create(store application.SessionStore, lifetime time.Duration) application.TokenIssue {
	fixture.t.Helper()
	issue := sessionTestIssue(fixture.t, lifetime)
	if err := store.Create(fixture.ctx, application.SessionFromIssue(issue), nil); err != nil {
		fixture.t.Fatal("create isolated login session failed")
	}
	fixture.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Revoke(ctx, sessionTestClaims(issue)); err != nil {
			fixture.t.Error("clean isolated login session failed")
		}
	})
	return issue
}

func (fixture *sessionRedisFixture) rotate(store application.SessionStore, credential application.TokenClaims, attempt string, candidate application.TokenIssue) application.TokenIssue {
	fixture.t.Helper()
	issue, err := store.Rotate(fixture.ctx, application.RefreshRotation{Credential: credential, AttemptID: attempt, Candidate: candidate})
	if err != nil {
		fixture.t.Fatal("rotate or recover isolated login session failed")
	}
	return issue
}

func assertSessionMaterial(t *testing.T, got, want application.TokenIssue) {
	t.Helper()
	// Do not print signing material, even when a regression fails.
	if !reflect.DeepEqual(got, want) {
		t.Fatal("fixed signing material changed instead of recovering the committed result")
	}
}

func (fixture *sessionRedisFixture) active(store application.SessionStore, want application.TokenIssue) {
	fixture.t.Helper()
	got, err := store.Read(fixture.ctx, want.SessionID)
	if err != nil {
		fixture.t.Fatal("expected an active login session")
	}
	if got.UserID != want.UserID || got.TokenVersion != want.TokenVersion || got.Generation != want.Generation ||
		got.RefreshDigest != application.CredentialDigest(want.RefreshID) || !got.ExpiresAt.Equal(want.RefreshExpiresAt) {
		fixture.t.Fatal("active session does not match the committed generation and original deadline")
	}
}

func (fixture *sessionRedisFixture) missing(store application.SessionStore, id string) {
	fixture.t.Helper()
	if _, err := store.Read(fixture.ctx, id); !errors.Is(err, application.ErrSessionMissing) {
		fixture.t.Fatal("expected a missing or revoked login session")
	}
}

func waitSessionDeadline(t *testing.T, ctx context.Context, deadline time.Time) {
	t.Helper()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("session integration deadline exceeded while waiting for real expiry")
	}
}

func testSessionConcurrentAttempt(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	const count = 12
	callers := make([]application.SessionStore, count)
	for index := range callers {
		callers[index] = fixture.caller()
	}
	initial := fixture.create(callers[0], time.Minute)
	credential, attempt := sessionTestClaims(initial), sessionTestID(t)
	candidates := make([]application.TokenIssue, count)
	results := make([]application.TokenIssue, count)
	errorsByCaller := make([]error, count)
	for index := range candidates {
		candidates[index] = sessionTestSuccessor(t, credential, 2*time.Minute)
	}
	var ready, finished sync.WaitGroup
	ready.Add(count)
	finished.Add(count)
	start := make(chan struct{})
	for index := range callers {
		go func(index int) {
			defer finished.Done()
			ready.Done()
			<-start
			results[index], errorsByCaller[index] = callers[index].Rotate(fixture.ctx, application.RefreshRotation{
				Credential: credential, AttemptID: attempt, Candidate: candidates[index],
			})
		}(index)
	}
	ready.Wait()
	close(start)
	finished.Wait()
	for index := range results {
		if errorsByCaller[index] != nil {
			t.Fatalf("independent concurrent caller %d failed", index)
		}
		assertSessionMaterial(t, results[index], results[0])
	}
	matches := 0
	for _, candidate := range candidates {
		if reflect.DeepEqual(results[0], candidate) {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("concurrent result must be exactly one of the independently proposed candidates")
	}
	fixture.active(callers[1], results[0])

	// A lost Cookie header retries with the predecessor; a lost body retries
	// with the already delivered successor Cookie. Neither may commit again.
	for index, retryCredential := range []application.TokenClaims{credential, sessionTestClaims(results[0])} {
		candidate := sessionTestSuccessor(t, retryCredential, 5*time.Minute)
		recovered := fixture.rotate(callers[index+2], retryCredential, attempt, candidate)
		assertSessionMaterial(t, recovered, results[0])
		fixture.active(callers[index+4], results[0])
	}
	// Recovery is also exercised without speculative signing material, as used
	// by the application's probe-before-prepare path.
	assertSessionMaterial(t, fixture.rotate(callers[6], credential, attempt, application.TokenIssue{}), results[0])
}

func testSessionRecoveryDeadline(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	first, second := fixture.caller(), fixture.caller()
	initial := fixture.create(first, 2*time.Minute)
	credential, attempt := sessionTestClaims(initial), sessionTestID(t)
	candidate := sessionTestSuccessor(t, credential, 3*time.Minute)
	committed := fixture.rotate(first, credential, attempt, candidate)
	// Anchor after the response, so the final retry is unambiguously beyond
	// the server's original 30 seconds, without assuming synchronized clocks.
	committedAt := time.Now()
	waitSessionDeadline(t, fixture.ctx, committedAt.Add(10*time.Second))
	assertSessionMaterial(t, fixture.rotate(second, credential, attempt, application.TokenIssue{}), committed)
	assertSessionMaterial(t, fixture.rotate(second, sessionTestClaims(committed), attempt, application.TokenIssue{}), committed)
	fixture.active(first, committed)
	waitSessionDeadline(t, fixture.ctx, committedAt.Add(30*time.Second+250*time.Millisecond))
	if _, err := second.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: credential, AttemptID: attempt, Candidate: sessionTestSuccessor(t, credential, 5*time.Minute),
	}); !errors.Is(err, application.ErrRefreshReuse) {
		t.Fatal("old credential outside the original recovery deadline must be reuse, not cached success")
	}
	fixture.missing(first, initial.SessionID)
}

func testSessionExpiry(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	first, second := fixture.caller(), fixture.caller()
	initial := fixture.create(first, 2*time.Second)
	credential, attempt := sessionTestClaims(initial), sessionTestID(t)
	candidate := sessionTestSuccessor(t, credential, 5*time.Second)
	committed := fixture.rotate(first, credential, attempt, candidate)
	fixture.active(second, committed)
	waitSessionDeadline(t, fixture.ctx, committed.RefreshExpiresAt.Add(-2*time.Second))
	assertSessionMaterial(t, fixture.rotate(second, credential, attempt, application.TokenIssue{}), committed)
	assertSessionMaterial(t, fixture.rotate(second, sessionTestClaims(committed), attempt, application.TokenIssue{}), committed)
	fixture.active(first, committed)
	waitSessionDeadline(t, fixture.ctx, committed.RefreshExpiresAt.Add(150*time.Millisecond))
	fixture.missing(second, committed.SessionID)
	for _, expiredCredential := range []application.TokenClaims{credential, sessionTestClaims(committed)} {
		if _, err := first.Rotate(fixture.ctx, application.RefreshRotation{
			Credential: expiredCredential, AttemptID: attempt, Candidate: sessionTestSuccessor(t, expiredCredential, time.Minute),
		}); !errors.Is(err, application.ErrSessionMissing) {
			t.Fatal("a still-cached attempt must not outlive its result or recreate an expired session")
		}
	}
	fixture.missing(first, committed.SessionID)
}

func testSessionExpiredPredecessor(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	first, second := fixture.caller(), fixture.caller()
	initial := fixture.create(first, 2*time.Second)
	credential, attempt := sessionTestClaims(initial), sessionTestID(t)
	committed := fixture.rotate(first, credential, attempt, sessionTestSuccessor(t, credential, time.Minute))
	waitSessionDeadline(t, fixture.ctx, initial.RefreshExpiresAt.Add(150*time.Millisecond))
	credential.Expired = true
	assertSessionMaterial(t, fixture.rotate(second, credential, attempt, application.TokenIssue{}), committed)
	fixture.active(first, committed)
	if _, err := second.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: credential, AttemptID: sessionTestID(t), Candidate: sessionTestSuccessor(t, credential, 2*time.Minute),
	}); err == nil {
		t.Fatal("an expired predecessor must not authorize an uncommitted attempt")
	}
	fixture.missing(first, initial.SessionID)
}

func testSessionSupersededAndReuse(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	first, second := fixture.caller(), fixture.caller()
	initial := fixture.create(first, time.Minute)
	independent := fixture.create(second, time.Minute)
	credential, attempt := sessionTestClaims(initial), sessionTestID(t)
	generationTwo := fixture.rotate(first, credential, attempt, sessionTestSuccessor(t, credential, 2*time.Minute))

	// A cached result cannot bypass the user/version binding, or invalidate a
	// legitimate session when a different binding is submitted.
	wrongUser, wrongVersion := credential, credential
	wrongUser.UserID++
	wrongVersion.TokenVersion++
	for _, invalid := range []application.TokenClaims{wrongUser, wrongVersion} {
		if _, err := second.Rotate(fixture.ctx, application.RefreshRotation{
			Credential: invalid, AttemptID: attempt, Candidate: sessionTestSuccessor(t, invalid, 3*time.Minute),
		}); !errors.Is(err, application.ErrSessionMissing) {
			t.Fatal("cached attempt accepted a mismatched user or authorization version")
		}
		fixture.active(first, generationTwo)
	}

	nextCredential := sessionTestClaims(generationTwo)
	generationThree := fixture.rotate(second, nextCredential, sessionTestID(t), sessionTestSuccessor(t, nextCredential, 3*time.Minute))
	for _, superseded := range []application.TokenClaims{credential, nextCredential} {
		if _, err := first.Rotate(fixture.ctx, application.RefreshRotation{
			Credential: superseded, AttemptID: attempt, Candidate: application.TokenIssue{},
		}); !errors.Is(err, application.ErrRefreshSuperseded) {
			t.Fatal("a confirmed attempt replaced by a higher generation must report superseded")
		}
		fixture.active(second, generationThree)
	}
	if _, err := first.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: credential, AttemptID: sessionTestID(t), Candidate: sessionTestSuccessor(t, credential, time.Minute),
	}); !errors.Is(err, application.ErrRefreshReuse) {
		t.Fatal("an older credential under an unrelated attempt must be abnormal reuse")
	}
	fixture.missing(second, initial.SessionID)
	fixture.active(first, independent)
	if _, err := second.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: nextCredential, AttemptID: attempt, Candidate: application.TokenIssue{},
	}); !errors.Is(err, application.ErrSessionMissing) {
		t.Fatal("reuse revocation must take precedence over a cached attempt")
	}
}

func testSessionLogoutInterleavings(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"logout_before_rotation", "rotation_before_logout", "simultaneous"} {
		t.Run(order, func(t *testing.T) {
			fixture := newSessionRedisFixture(t)
			rotator, revoker, observer := fixture.caller(), fixture.caller(), fixture.caller()
			initial := fixture.create(observer, time.Minute)
			independent := fixture.create(observer, time.Minute)
			credential, attempt := sessionTestClaims(initial), sessionTestID(t)
			candidate := sessionTestSuccessor(t, credential, 2*time.Minute)
			var rotationErr, logoutErr error
			var committed application.TokenIssue
			var ready, finished sync.WaitGroup
			ready.Add(2)
			finished.Add(2)
			start, rotationDone, logoutDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
			go func() {
				defer finished.Done()
				defer close(rotationDone)
				ready.Done()
				<-start
				if order == "logout_before_rotation" {
					<-logoutDone
				}
				committed, rotationErr = rotator.Rotate(fixture.ctx, application.RefreshRotation{
					Credential: credential, AttemptID: attempt, Candidate: candidate,
				})
			}()
			go func() {
				defer finished.Done()
				defer close(logoutDone)
				ready.Done()
				<-start
				if order == "rotation_before_logout" {
					<-rotationDone
				}
				logoutErr = revoker.Revoke(fixture.ctx, credential)
			}()
			ready.Wait()
			close(start)
			finished.Wait()
			if logoutErr != nil {
				t.Fatal("logout failed during the controlled interleaving")
			}
			switch order {
			case "logout_before_rotation":
				if !errors.Is(rotationErr, application.ErrSessionMissing) {
					t.Fatal("rotation after completed logout must observe the terminal session")
				}
			case "rotation_before_logout":
				if rotationErr != nil {
					t.Fatal("rotation before logout must commit normally")
				}
				assertSessionMaterial(t, committed, candidate)
			default:
				if rotationErr != nil && !errors.Is(rotationErr, application.ErrSessionMissing) {
					t.Fatal("concurrent rotation must either commit before logout or observe missing")
				}
			}
			fixture.missing(observer, initial.SessionID)
			for _, retryCredential := range []application.TokenClaims{credential, sessionTestClaims(candidate)} {
				if _, err := observer.Rotate(fixture.ctx, application.RefreshRotation{
					Credential: retryCredential, AttemptID: attempt, Candidate: sessionTestSuccessor(t, retryCredential, 3*time.Minute),
				}); !errors.Is(err, application.ErrSessionMissing) {
					t.Fatal("neither the old nor newly issued Cookie may revive a logged-out session")
				}
			}
			if err := revoker.Revoke(fixture.ctx, credential); err != nil {
				t.Fatal("repeated logout must remain idempotent")
			}
			fixture.missing(observer, initial.SessionID)
			fixture.active(observer, independent)
		})
	}
}

func testSessionRedisFailure(t *testing.T) {
	t.Parallel()
	fixture := newSessionRedisFixture(t)
	store := fixture.caller()
	absent := sessionTestIssue(t, time.Minute)
	credential := sessionTestClaims(absent)
	fixture.missing(store, absent.SessionID)
	if _, err := store.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: credential, AttemptID: sessionTestID(t), Candidate: sessionTestSuccessor(t, credential, time.Minute),
	}); !errors.Is(err, application.ErrSessionMissing) {
		t.Fatal("successful Redis lookup of an absent session must report missing")
	}
	if err := store.Revoke(fixture.ctx, credential); err != nil {
		t.Fatal("revoking an absent session must succeed idempotently")
	}
	fixture.missing(store, absent.SessionID)
	active := fixture.create(store, time.Minute)

	// Inject an actual server-side command failure, not a fake error or a
	// locally closed client. The dedicated temporary ACL user can PING but
	// Redis itself refuses its session reads and atomic scripts. Requires an
	// isolated integration Redis whose administrative user can manage ACLs.
	username, password := "session-gate-"+sessionTestID(t), sessionTestID(t)
	if err := fixture.admin.Do(fixture.ctx, "ACL", "SETUSER", username, "reset", "on", ">"+password,
		"~*", "+@all", "-hgetall", "-eval", "-evalsha").Err(); err != nil {
		t.Fatal("prepare temporary Redis query-failure ACL user failed; use the isolated Redis gate")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fixture.admin.Do(ctx, "ACL", "DELUSER", username).Err(); err != nil {
			t.Error("remove temporary Redis query-failure ACL user failed")
		}
	})
	options := fixture.options
	options.Username, options.Password = username, password
	failing := redisadapter.NewStore(fixture.client(options))
	_, readErr := failing.Read(fixture.ctx, active.SessionID)
	_, rotationErr := failing.Rotate(fixture.ctx, application.RefreshRotation{
		Credential: sessionTestClaims(active), AttemptID: sessionTestID(t),
		Candidate: sessionTestSuccessor(t, sessionTestClaims(active), 2*time.Minute),
	})
	logoutErr := failing.Revoke(fixture.ctx, sessionTestClaims(active))
	for _, err := range []error{readErr, rotationErr, logoutErr} {
		if err == nil || errors.Is(err, application.ErrSessionMissing) || errors.Is(err, application.ErrRefreshReuse) ||
			errors.Is(err, application.ErrRefreshSuperseded) || !strings.Contains(err.Error(), "NOPERM") {
			t.Fatal("real Redis command refusal must remain an infrastructure error, not missing/reuse/success")
		}
	}
	fixture.active(store, active)
}
