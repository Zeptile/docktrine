package deploy

import (
	"context"

	"github.com/docker/docker/client"
)

// HelperImage is the image of the short-lived helper containers Docktrine
// runs on nodes. Other packages use it for their own helpers so a node only
// ever needs the one image.
const HelperImage = helperImage

// EnsureHelperImage pulls the helper image on the node if it's missing.
func EnsureHelperImage(ctx context.Context, cli *client.Client) error {
	return ensureImage(ctx, cli, helperImage)
}
