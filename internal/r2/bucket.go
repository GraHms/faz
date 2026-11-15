package r2

import (
	"context"
	"net/http"
	"strings"
)

// CreateBucket creates the client's bucket. Production buckets are created
// once in the Cloudflare dashboard or with wrangler; this exists for tests
// and local development against an S3 compatible server.
func CreateBucket(ctx context.Context, c *Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(c.Endpoint, "/")+"/"+c.Bucket, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	if err != nil && strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		return nil
	}
	return err
}
