package tokenbucket

import "errors"

var (
	ErrInitializedBucketFailed = errors.New("failed to initialize token bucket")
	ErrNoTokensAvailable       = errors.New("no tokens available in the bucket")
)
