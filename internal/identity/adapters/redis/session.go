package redisadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"admin/internal/identity/application"
	"admin/internal/identity/domain"
	"github.com/redis/go-redis/v9"
)

const sessionPrefix = "auth:session:"
const attemptPrefix = "auth:refresh-attempt:"

var createSession = redis.NewScript(`
local now = redis.call('TIME')
local ms = now[1] * 1000 + math.floor(now[2] / 1000)
if tonumber(ARGV[5]) <= ms then return 0 end
if redis.call('EXISTS', KEYS[1]) ~= 0 then return redis.error_reply('session collision') end
redis.call('HSET', KEYS[1], 'user', ARGV[1], 'version', ARGV[2], 'generation', ARGV[3], 'digest', ARGV[4], 'expires', ARGV[5])
redis.call('PEXPIREAT', KEYS[1], ARGV[5])
if ARGV[6] == '1' and redis.call('HGET', KEYS[2], 'user') == ARGV[7] and redis.call('HGET', KEYS[2], 'version') == ARGV[8] and redis.call('HGET', KEYS[2], 'generation') == ARGV[9] and redis.call('HGET', KEYS[2], 'digest') == ARGV[10] then
 redis.call('DEL', KEYS[2])
end
return 1
`)

func (store *Store) Create(ctx context.Context, session domain.LoginSession, previous *application.TokenClaims) error {
	if !application.ValidSessionIdentifier(session.ID) || session.UserID == 0 || session.TokenVersion <= 0 || session.Generation < 1 || len(session.RefreshDigest) != 64 {
		return errors.New("invalid login session")
	}
	oldKey, enabled, user, version := sessionPrefix+session.ID, "0", uint(0), 0
	oldGeneration, oldDigest := int64(0), ""
	if previous != nil && application.ValidSessionIdentifier(previous.SessionID) && previous.SessionID != session.ID && previous.Purpose == application.TokenPurposeRefresh && !previous.Expired && previous.ExpiresAt.After(time.Now()) {
		oldKey, enabled, user, version = sessionPrefix+previous.SessionID, "1", previous.UserID, previous.TokenVersion
		oldGeneration, oldDigest = previous.Generation, application.CredentialDigest(previous.RefreshID)
	}
	result, err := createSession.Run(ctx, store.client, []string{sessionPrefix + session.ID, oldKey}, session.UserID, session.TokenVersion, session.Generation, session.RefreshDigest, session.ExpiresAt.UnixMilli(), enabled, user, version, oldGeneration, oldDigest).Int()
	if err != nil {
		return err
	}
	if result != 1 {
		return errors.New("login session expired before commit")
	}
	return nil
}

func (store *Store) Read(ctx context.Context, id string) (domain.LoginSession, error) {
	if !application.ValidSessionIdentifier(id) {
		return domain.LoginSession{}, application.ErrSessionMissing
	}
	values, err := store.client.HGetAll(ctx, sessionPrefix+id).Result()
	if err != nil {
		return domain.LoginSession{}, err
	}
	if len(values) == 0 {
		return domain.LoginSession{}, application.ErrSessionMissing
	}
	user, e1 := strconv.ParseUint(values["user"], 10, 64)
	version, e2 := strconv.Atoi(values["version"])
	generation, e3 := strconv.ParseInt(values["generation"], 10, 64)
	expires, e4 := strconv.ParseInt(values["expires"], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || user == 0 || version <= 0 || generation < 1 || len(values["digest"]) != 64 {
		return domain.LoginSession{}, errors.New("invalid stored login session")
	}
	if expires <= time.Now().UnixMilli() {
		return domain.LoginSession{}, application.ErrSessionMissing
	}
	return domain.LoginSession{ID: id, UserID: uint(user), TokenVersion: version, Generation: generation, RefreshDigest: values["digest"], ExpiresAt: time.UnixMilli(expires)}, nil
}

var rotateSession = redis.NewScript(`
local now = redis.call('TIME')
local ms = now[1] * 1000 + math.floor(now[2] / 1000)
local user = redis.call('HGET', KEYS[1], 'user')
if not user or user ~= ARGV[1] or redis.call('HGET', KEYS[1], 'version') ~= ARGV[2] then return {0} end
local generation = tonumber(redis.call('HGET', KEYS[1], 'generation'))
local digest = redis.call('HGET', KEYS[1], 'digest')
local result = redis.call('HGET', KEYS[2], 'material')
if result then
 local previous = redis.call('HGET', KEYS[2], 'previous')
 local nextDigest = redis.call('HGET', KEYS[2], 'next')
 local nextGeneration = tonumber(redis.call('HGET', KEYS[2], 'generation'))
 local bound = (ARGV[3] == previous and tonumber(ARGV[4]) == nextGeneration - 1) or (ARGV[3] == nextDigest and tonumber(ARGV[4]) == nextGeneration)
 if bound then
  if generation > nextGeneration then return {-3} end
  if generation == nextGeneration and digest == nextDigest and tonumber(redis.call('HGET', KEYS[2], 'deadline')) > ms and tonumber(redis.call('HGET', KEYS[2], 'expires')) > ms then return {1, result} end
 end
 redis.call('DEL', KEYS[1])
 return {-2}
end
if digest ~= ARGV[3] or generation ~= tonumber(ARGV[4]) then
 redis.call('DEL', KEYS[1])
 return {-2}
end
-- An expired predecessor can recover a committed result above, never commit.
if tonumber(ARGV[5]) <= ms then return {0} end
if ARGV[6] == '' then return {-4} end
if tonumber(ARGV[8]) ~= generation + 1 or tonumber(ARGV[9]) <= ms then return redis.error_reply('invalid rotation material') end
redis.call('HSET', KEYS[1], 'generation', ARGV[8], 'digest', ARGV[7], 'expires', ARGV[9])
redis.call('PEXPIREAT', KEYS[1], ARGV[9])
redis.call('HSET', KEYS[2], 'previous', ARGV[3], 'next', ARGV[7], 'generation', ARGV[8], 'material', ARGV[6], 'deadline', ms + 30000, 'expires', ARGV[9])
redis.call('PEXPIREAT', KEYS[2], ms + 30000)
return {1, ARGV[6]}
`)

func (store *Store) Rotate(ctx context.Context, request application.RefreshRotation) (application.TokenIssue, error) {
	credential, candidate := request.Credential, request.Candidate
	if !application.ValidSessionIdentifier(credential.SessionID) || !application.ValidSessionIdentifier(request.AttemptID) || !application.ValidSessionIdentifier(credential.RefreshID) || credential.Purpose != application.TokenPurposeRefresh {
		return application.TokenIssue{}, application.ErrSessionMissing
	}
	var material []byte
	nextDigest := ""
	if candidate.SessionID != "" {
		if candidate.SessionID != credential.SessionID || candidate.UserID != credential.UserID || candidate.TokenVersion != credential.TokenVersion || candidate.Generation != credential.Generation+1 || !application.ValidSessionIdentifier(candidate.RefreshID) || !candidate.AccessExpiresAt.After(candidate.IssuedAt) || !candidate.RefreshExpiresAt.After(candidate.IssuedAt) {
			return application.TokenIssue{}, errors.New("invalid rotation binding")
		}
		var err error
		material, err = json.Marshal(candidate)
		if err != nil {
			return application.TokenIssue{}, err
		}
		nextDigest = application.CredentialDigest(candidate.RefreshID)
	}
	result, err := rotateSession.Run(ctx, store.client, []string{sessionPrefix + credential.SessionID, attemptPrefix + credential.SessionID + ":" + request.AttemptID}, credential.UserID, credential.TokenVersion, application.CredentialDigest(credential.RefreshID), credential.Generation, credential.ExpiresAt.UnixMilli(), string(material), nextDigest, candidate.Generation, candidate.RefreshExpiresAt.UnixMilli()).Slice()
	if err != nil {
		return application.TokenIssue{}, err
	}
	if len(result) == 0 {
		return application.TokenIssue{}, errors.New("invalid rotation result")
	}
	switch result[0] {
	case int64(0):
		return application.TokenIssue{}, application.ErrSessionMissing
	case int64(-2):
		return application.TokenIssue{}, application.ErrRefreshReuse
	case int64(-3):
		return application.TokenIssue{}, application.ErrRefreshSuperseded
	case int64(-4):
		return application.TokenIssue{}, application.ErrRefreshUncommitted
	case int64(1):
		if len(result) != 2 {
			return application.TokenIssue{}, errors.New("missing fixed rotation result")
		}
		value, ok := result[1].(string)
		if !ok {
			return application.TokenIssue{}, errors.New("invalid fixed rotation result")
		}
		var issue application.TokenIssue
		if err := json.Unmarshal([]byte(value), &issue); err != nil {
			return application.TokenIssue{}, fmt.Errorf("decode fixed rotation result: %w", err)
		}
		return issue, nil
	default:
		return application.TokenIssue{}, errors.New("unknown rotation result")
	}
}

var revokeSession = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'user') == ARGV[1] and redis.call('HGET', KEYS[1], 'version') == ARGV[2] then redis.call('DEL', KEYS[1]) end
return 1
`)

func (store *Store) Revoke(ctx context.Context, credential application.TokenClaims) error {
	if !application.ValidSessionIdentifier(credential.SessionID) || credential.Purpose != application.TokenPurposeRefresh {
		return nil
	}
	return revokeSession.Run(ctx, store.client, []string{sessionPrefix + credential.SessionID}, credential.UserID, credential.TokenVersion).Err()
}
