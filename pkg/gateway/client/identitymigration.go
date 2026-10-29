package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

const (
	// identityGroupLookupIDHashesMigrationName names the backfill of the hashed group lookup IDs of identities.
	identityGroupLookupIDHashesMigrationName = "identity_group_lookup_id_hashes"

	// identityGroupLookupIDBackfillBatchSize bounds the identities that each step of the backfill reads and writes.
	identityGroupLookupIDBackfillBatchSize = 500
)

// BackfillIdentityGroupLookupIDHashes stores the hash of the group lookup ID of every identity that was written
// without one, once. Identities written since the column was added already have it, because encryptIdentity sets it.
//
// The backfill reads the identities in batches, in primary key order, and holds no transaction between them. It is
// idempotent, so replicas starting together may both run it, and a run that fails is retried from the start.
func (c *Client) BackfillIdentityGroupLookupIDHashes(ctx context.Context) error {
	return c.MigrateKinmIfNotRun(ctx, identityGroupLookupIDHashesMigrationName, func() error {
		return c.backfillIdentityGroupLookupIDHashes(ctx, identityGroupLookupIDBackfillBatchSize)
	})
}

func (c *Client) backfillIdentityGroupLookupIDHashes(ctx context.Context, batchSize int) error {
	var (
		db      = c.db.WithContext(ctx)
		after   *types.Identity
		scanned int
		updated int64
	)
	for {
		query := db.Where("hashed_provider_group_lookup_id IS NULL OR hashed_provider_group_lookup_id = ''").
			Order("auth_provider_namespace, auth_provider_name, hashed_provider_user_id").
			Limit(batchSize)
		if after != nil {
			query = query.Where("(auth_provider_namespace, auth_provider_name, hashed_provider_user_id) > (?, ?, ?)",
				after.AuthProviderNamespace, after.AuthProviderName, after.HashedProviderUserID)
		}

		var identities []types.Identity
		if err := query.Find(&identities).Error; err != nil {
			return fmt.Errorf("failed to list identities: %w", err)
		}
		if len(identities) == 0 {
			break
		}
		last := identities[len(identities)-1]
		after = &last

		if err := db.Transaction(func(tx *gorm.DB) error {
			for i := range identities {
				identity := &identities[i]
				// The lookup ID as stored, which may be encrypted, so that the write below applies only if no one has
				// changed it since it was read.
				stored := identity.ProviderGroupLookupID
				if identity.Encrypted && (c.encryptionConfig == nil || c.encryptionConfig.Transformers[identityGroupResource] == nil) {
					// decryptIdentity would leave the lookup ID encrypted, and its hash would match nothing.
					return fmt.Errorf("identity is encrypted but encryption is not configured")
				}
				if err := c.decryptIdentity(ctx, identity); err != nil {
					return fmt.Errorf("failed to decrypt identity: %w", err)
				}
				if identity.ProviderGroupLookupID == "" {
					continue
				}

				result := tx.Model(new(types.Identity)).
					Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?",
						identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID).
					Where("provider_group_lookup_id = ?", stored).
					Where("hashed_provider_group_lookup_id IS NULL OR hashed_provider_group_lookup_id = ''").
					UpdateColumn("hashed_provider_group_lookup_id", hash.String(identity.ProviderGroupLookupID))
				if result.Error != nil {
					return fmt.Errorf("failed to store the hashed group lookup ID: %w", result.Error)
				}
				updated += result.RowsAffected
			}
			return nil
		}); err != nil {
			return err
		}

		scanned += len(identities)
		slog.Info("Backfilling hashed identity group lookup IDs", "scanned", scanned, "updated", updated)
		if len(identities) < batchSize {
			break
		}
	}

	if scanned > 0 {
		slog.Info("Backfilled hashed identity group lookup IDs", "scanned", scanned, "updated", updated)
	}
	return nil
}
