package domain

import "time"

// LoginSession is Identity's revocable login fact, independent of access versions.
// RefreshDigest is a SHA-256 credential identifier digest, never a JWT.
type LoginSession struct {
	ID            string
	UserID        uint
	TokenVersion  int
	Generation    int64
	RefreshDigest string
	ExpiresAt     time.Time
}
