package cli

import (
	"context"

	"db-sync/internal/model"
)

func SaveReviewedProfile(ctx context.Context, app *App, reviewed model.Profile) error {
	return app.SaveReviewedProfile(ctx, reviewed)
}
