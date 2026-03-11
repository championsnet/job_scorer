package multitenant

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
)

func OpenFirestore(ctx context.Context, cfg *RuntimeConfig) (*firestore.Client, error) {
	projectID := cfg.FirestoreProjectID
	if projectID == "" {
		projectID = cfg.FirebaseProjectID
	}
	if projectID == "" {
		projectID = cfg.CloudTasksProjectID
	}
	if projectID == "" {
		return nil, fmt.Errorf("FIRESTORE_PROJECT_ID or FIREBASE_PROJECT_ID is required for Firestore runtime")
	}
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed creating Firestore client: %w", err)
	}
	return client, nil
}
