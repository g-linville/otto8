package client

import (
	"crypto/aes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	encryptaes "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

const (
	lookupTestProviderName = "lookup-test-auth-provider"
	lookupTestGroupPrefix  = "lookup/"
)

var (
	lookupTestProvider = AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      lookupTestProviderName,
	}
)

// lookupIDStub stands in for an auth provider whose user info reports the access token as the user's group lookup ID,
// so that each test sign-in chooses its own.
type lookupIDStub struct {
	*httptest.Server
	// failing makes the user info endpoint fail.
	failing atomic.Bool
}

func newLookupIDStub(t *testing.T) *lookupIDStub {
	t.Helper()

	stub := new(lookupIDStub)
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/obot-get-user-info":
			if stub.failing.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
				"name": "Login Name",
			})
		case "/obot-list-user-auth-groups":
			_ = json.NewEncoder(w).Encode([]auth.GroupInfo{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(stub.Close)
	return stub
}

// newLookupTestClient returns a client that encrypts identities and users, for an auth provider whose adapter matches
// SCIM users by group lookup ID, binds groups by externalId, and deletes users.
func newLookupTestClient(t *testing.T) *Client {
	t.Helper()

	adapter.RegisterForTest(t, lookupTestProviderName, adapter.TestAdapter{
		Users:  adapter.UserKeyGroupLookupID,
		Groups: adapter.GroupKeyExternalID,
		Shared: adapter.Rules{
			DeletesUsers: true,
		},
	})

	c := newLifecycleTestClient(t)
	block, err := aes.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	transformer, err := encryptaes.NewGCMTransformer(block)
	require.NoError(t, err)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			identityGroupResource: transformer,
			userGroupResource:     transformer,
		},
	}
	return c
}

func createLookupTestConnection(t *testing.T, c *Client) *types.SCIMConnection {
	t.Helper()

	conn, _, err := c.CreateSCIMConnection(t.Context(), CreateSCIMConnectionOptions{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		GroupIDPrefix:         lookupTestGroupPrefix,
		Issuer:                "tenant",
		Origin:                types.SCIMConnectionOriginMigrated,
		IssueToken:            true,
	})
	require.NoError(t, err)
	return conn
}

// lookupSignIn signs in through the lookup test provider, with subject sub, as the person whose group lookup ID is
// objectID.
func lookupSignIn(t *testing.T, c *Client, stub *lookupIDStub, sub, objectID string) (*types.User, error) {
	t.Helper()

	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), stub.URL), lookupTestGroupPrefix), objectID)
	return c.EnsureIdentity(ctx, &types.Identity{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		ProviderUsername:      sub,
		ProviderUserID:        sub,
		Email:                 sub + "@example.com",
	}, "", UserLimit{
		Unlimited: true,
	})
}

func provisionLookupTestUser(t *testing.T, c *Client, conn *types.SCIMConnection, objectID string) (*SCIMUser, error) {
	t.Helper()

	return c.CreateSCIMUser(t.Context(), conn, SCIMUserInput{
		UserName:   objectID + "@example.com",
		ExternalID: objectID,
		Profile: types.SCIMUserProfile{
			DisplayName: "SCIM Name",
		},
	}, SCIMUserCreateOptions{
		UserLimit: UserLimit{
			Unlimited: true,
		},
		DefaultRole: apitypes.RoleBasic,
	})
}

// identityCount returns the number of identities of the lookup test provider with the given provider user ID.
func identityCount(t *testing.T, c *Client, providerUserID string) int64 {
	t.Helper()

	var n int64
	require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.Identity)).
		Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?",
			lookupTestProvider.Namespace, lookupTestProvider.Name, hash.String(providerUserID)).
		Count(&n).Error)
	return n
}

func decryptedUser(t *testing.T, c *Client, userID uint) types.User {
	t.Helper()

	user := storedLifecycleUser(t, c, userID)
	require.NoError(t, c.decryptUser(t.Context(), &user))
	return user
}

func TestSCIMFirstUserIsClaimedAtFirstSignIn(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	// SCIM creates the user before they ever sign in, with an identity that waits for their subject.
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-1")
	require.NoError(t, err)
	require.EqualValues(t, 1, identityCount(t, c, adapter.PendingProviderUserID("oid-1")))
	require.Equal(t, "oid-1", decryptedUser(t, c, provisioned.UserID).Username)

	// SCIM is enforced, which a claimed identity satisfies.
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)

	// The first sign-in claims the identity: it is keyed by the subject, and its user gets the subject as username.
	user, err := lookupSignIn(t, c, stub, "sub-1", "oid-1")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, user.ID)
	require.Zero(t, identityCount(t, c, adapter.PendingProviderUserID("oid-1")))
	require.EqualValues(t, 1, identityCount(t, c, "sub-1"))

	stored := *storedIdentity(t, c, lookupTestProvider, "sub-1")
	require.True(t, stored.Encrypted)
	require.Equal(t, hash.String("oid-1"), stored.HashedProviderGroupLookupID)
	require.NotNil(t, stored.FirstSignInAt)
	require.NoError(t, c.decryptIdentity(t.Context(), &stored))
	require.Equal(t, "sub-1", stored.ProviderUserID)
	require.Equal(t, "sub-1", stored.ProviderUsername)
	require.Equal(t, "oid-1", stored.ProviderGroupLookupID)

	renamed := decryptedUser(t, c, provisioned.UserID)
	require.Equal(t, "sub-1", renamed.Username)
	require.Equal(t, hash.String("sub-1"), renamed.HashedUsername)
	// The rest of the user is encrypted again with the new username's context, and still reads.
	require.Equal(t, "SCIM Name", renamed.DisplayName)

	// Later sign-ins find the identity by the subject.
	again, err := lookupSignIn(t, c, stub, "sub-1", "oid-1")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, again.ID)

	// SCIM still finds the user by the group lookup ID.
	got, err := c.GetSCIMUser(t.Context(), conn.ID, provisioned.ID)
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, got.UserID)
}

func TestSCIMCreateBindsTheIdentityThatSignInCreated(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	// A sign-in before SCIM provisions the person stores the group lookup ID with the identity.
	signedIn, err := lookupSignIn(t, c, stub, "sub-2", "oid-2")
	require.NoError(t, err)
	require.Equal(t, hash.String("oid-2"), storedIdentity(t, c, lookupTestProvider, "sub-2").HashedProviderGroupLookupID)

	// SCIM binds to that user rather than creating another.
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-2")
	require.NoError(t, err)
	require.Equal(t, signedIn.ID, provisioned.UserID)
	require.Zero(t, identityCount(t, c, adapter.PendingProviderUserID("oid-2")))

	// A second create of the same person is a conflict.
	_, err = provisionLookupTestUser(t, c, conn, "oid-2")
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)
}

func TestSCIMManagedSignInFillsAMissingGroupLookupID(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)

	// An identity from before group lookup IDs were stored has none.
	signedIn, err := lookupSignIn(t, c, stub, "sub-3", "oid-3")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.Identity)).Where("user_id = ?", signedIn.ID).
		UpdateColumns(map[string]any{
			"provider_group_lookup_id":        "",
			"hashed_provider_group_lookup_id": "",
			"encrypted":                       false,
			"provider_username":               "sub-3",
			"provider_user_id":                "sub-3",
			"email":                           "sub-3@example.com",
			"icon_url":                        "",
		}).Error)

	// Once SCIM manages the provider, the auth provider is not asked for groups, but the lookup ID is still filled
	// in, so that SCIM can find the user.
	conn := createLookupTestConnection(t, c)
	_, err = lookupSignIn(t, c, stub, "sub-3", "oid-3")
	require.NoError(t, err)
	require.Equal(t, hash.String("oid-3"), storedIdentity(t, c, lookupTestProvider, "sub-3").HashedProviderGroupLookupID)

	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-3")
	require.NoError(t, err)
	require.Equal(t, signedIn.ID, provisioned.UserID)
}

func TestFirstSignInFailsWithoutAGroupLookupIDOnlyWithAConnection(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	stub.failing.Store(true)

	// Without a connection, there is nothing to claim, so the sign-in goes ahead.
	_, err := lookupSignIn(t, c, stub, "sub-4", "oid-4")
	require.NoError(t, err)

	// With one, a first sign-in that cannot learn its group lookup ID could duplicate a provisioned user.
	createLookupTestConnection(t, c)
	_, err = lookupSignIn(t, c, stub, "sub-5", "oid-5")
	require.ErrorContains(t, err, "group lookup ID")
	require.Zero(t, identityCount(t, c, "sub-5"))

	// A sign-in whose identity exists does not need it.
	_, err = lookupSignIn(t, c, stub, "sub-4", "oid-4")
	require.NoError(t, err)
}

func TestSCIMCreateRefusesDuplicateUsersWithTheSameGroupLookupID(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)

	// Two users have identities with the same group lookup ID, as a legacy duplicate does.
	_, err := lookupSignIn(t, c, stub, "sub-6a", "oid-6")
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-6b", "oid-6")
	require.NoError(t, err)

	conn := createLookupTestConnection(t, c)
	_, err = provisionLookupTestUser(t, c, conn, "oid-6")
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)
	require.Contains(t, conflict.Message, "duplicates")
}

func TestSCIMCreateStartsANewAccountForADeletedUser(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)

	signedIn, err := lookupSignIn(t, c, stub, "sub-7", "oid-7")
	require.NoError(t, err)
	require.NoError(t, c.DeleteUser(t.Context(), strconv.FormatUint(uint64(signedIn.ID), 10)))

	conn := createLookupTestConnection(t, c)
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-7")
	require.NoError(t, err)
	require.NotEqual(t, signedIn.ID, provisioned.UserID)

	// The identity moved to the new account, which the person's next sign-in reaches.
	require.Equal(t, provisioned.UserID, storedIdentity(t, c, lookupTestProvider, "sub-7").UserID)
	user, err := lookupSignIn(t, c, stub, "sub-7", "oid-7")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, user.ID)
}

func TestDeleteSCIMUser(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	// An owner who stays, so that the deleted user is not the last one.
	owner, err := lookupSignIn(t, c, stub, "sub-owner", "oid-owner")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", owner.ID).UpdateColumn("role", apitypes.RoleOwner).Error)

	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-8")
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-8", "oid-8")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.AuthToken{
		ID:                    "token-8",
		UserID:                provisioned.UserID,
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		HashedToken:           hash.String("token-8"),
	}).Error)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
		ID:                    lookupTestGroupPrefix + "g-8",
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		Name:                  "team",
	}).Error)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.GroupMemberships{
		UserID:  provisioned.UserID,
		GroupID: lookupTestGroupPrefix + "g-8",
	}).Error)

	userID, err := c.DeleteSCIMUser(ctx, conn, provisioned.ID)
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, userID)

	// The user is disabled and deleted, their tokens are gone, and their SCIM ID is retired, all at once.
	deleted := storedLifecycleUser(t, c, userID)
	require.NotNil(t, deleted.DeletedAt)
	require.NotNil(t, deleted.DisabledAt)
	require.Equal(t, types.UserDisabledReasonSCIMDeleted, deleted.DisabledReason)
	require.Zero(t, rowCount(t, c, new(types.AuthToken), "user_id = ?", userID))
	require.Zero(t, rowCount(t, c, new(types.GroupMemberships), "user_id = ?", userID))
	_, err = c.GetSCIMUser(ctx, conn.ID, provisioned.ID)
	var notFound *SCIMNotFoundError
	require.ErrorAs(t, err, &notFound)

	// The deletion event creates one cleanup, however often it is delivered.
	var events []types.UserLifecycleEvent
	require.NoError(t, c.db.WithContext(ctx).Where("user_id = ? AND type = ?", userID, types.UserLifecycleEventDeleted).Find(&events).Error)
	require.Len(t, events, 1)
	require.NoError(t, c.deliverUserLifecycleEvent(ctx, events[0], c.listOAuthTokensOnce(ctx)))
	require.NoError(t, c.deliverUserLifecycleEvent(ctx, events[0], c.listOAuthTokensOnce(ctx)))
	var cleanups v1.UserDeleteList
	require.NoError(t, c.storageClient.List(ctx, &cleanups))
	require.Len(t, cleanups.Items, 1)
	require.Equal(t, userID, cleanups.Items[0].Spec.UserID)

	// A retired or unknown SCIM ID is not found.
	_, err = c.DeleteSCIMUser(ctx, conn, provisioned.ID)
	require.ErrorAs(t, err, &notFound)
	_, err = c.DeleteSCIMUser(ctx, conn, "unknown")
	require.ErrorAs(t, err, &notFound)

	// Provisioning the same person again starts a new account.
	again, err := provisionLookupTestUser(t, c, conn, "oid-8")
	require.NoError(t, err)
	require.NotEqual(t, userID, again.UserID)
}

func TestDeleteSCIMUserRefusesTheLastOwnerAndAdmin(t *testing.T) {
	for name, role := range map[string]apitypes.Role{
		"owner": apitypes.RoleOwner,
		"admin": apitypes.RoleAdmin,
	} {
		t.Run(name, func(t *testing.T) {
			c := newLookupTestClient(t)
			conn := createLookupTestConnection(t, c)

			provisioned, err := provisionLookupTestUser(t, c, conn, "oid-9")
			require.NoError(t, err)
			require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.User)).Where("id = ?", provisioned.UserID).UpdateColumns(map[string]any{
				"role":         role,
				"hashed_email": hash.String("oid-9@example.com"),
			}).Error)

			_, err = c.DeleteSCIMUser(t.Context(), conn, provisioned.ID)
			var conflict *SCIMConflictError
			require.ErrorAs(t, err, &conflict)

			// Nothing changed.
			user := storedLifecycleUser(t, c, provisioned.UserID)
			require.Nil(t, user.DeletedAt)
			require.Nil(t, user.DisabledAt)
			_, err = c.GetSCIMUser(t.Context(), conn.ID, provisioned.ID)
			require.NoError(t, err)
		})
	}
}

func TestAdminDeletionRecordsTheCleanupEvent(t *testing.T) {
	c := newLifecycleTestClient(t)
	user := createLifecycleTestUser(t, c, "deleted", lifecycleTestProvider)

	require.NoError(t, c.DeleteUser(t.Context(), strconv.FormatUint(uint64(user.ID), 10)))
	var events []types.UserLifecycleEvent
	require.NoError(t, c.db.WithContext(t.Context()).Where("user_id = ? AND type = ?", user.ID, types.UserLifecycleEventDeleted).Find(&events).Error)
	require.Len(t, events, 1)

	require.NoError(t, c.deliverUserLifecycleEvents(t.Context()))
	var cleanups v1.UserDeleteList
	require.NoError(t, c.storageClient.List(t.Context(), &cleanups))
	require.Len(t, cleanups.Items, 1)
	require.Equal(t, user.ID, cleanups.Items[0].Spec.UserID)

	// A refused deletion records nothing.
	require.Error(t, c.DeleteUser(t.Context(), "999"))
	require.Zero(t, rowCount(t, c, new(types.UserLifecycleEvent), "user_id = ?", 999))
}

func TestSCIMGroupsBindByExternalID(t *testing.T) {
	c := newLookupTestClient(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	// A group that directory synchronization created, with a grant.
	legacyID := lookupTestGroupPrefix + "g-1"
	require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
		ID:                    legacyID,
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		Name:                  "Old Name",
	}).Error)
	_, err := c.CreateGroupRoleAssignment(ctx, legacyID, apitypes.RoleAdmin, "")
	require.NoError(t, err)

	// The pushed group binds to it by externalId, keeps its ID, and takes the pushed name.
	group, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team",
		ExternalID:  "g-1",
	})
	require.NoError(t, err)
	require.Equal(t, legacyID, group.GroupID)
	require.Equal(t, "g-1", group.ExternalID)
	require.Equal(t, "Team", group.DisplayName)

	// It is found by its externalId, as the adapter normalizes it.
	for _, externalID := range []string{"g-1", " g-1 "} {
		found, total, err := c.ListSCIMGroups(ctx, conn, SCIMGroupFilter{
			ExternalID: externalID,
		}, SCIMPage{
			Limit: 10,
		}, false)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Equal(t, group.ID, found[0].ID)
		require.Equal(t, "g-1", found[0].ExternalID)
	}
	found, total, err := c.ListSCIMGroups(ctx, conn, SCIMGroupFilter{
		ExternalID: "g-other",
	}, SCIMPage{
		Limit: 10,
	}, false)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, found)

	// Pushing the same group again is a conflict.
	_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team Again",
		ExternalID:  "g-1",
	})
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)

	// A group without an existing one is created with the ID that its externalId gives.
	created, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "New Team",
		ExternalID:  "g-2",
	})
	require.NoError(t, err)
	require.Equal(t, lookupTestGroupPrefix+"g-2", created.GroupID)

	// A group without an externalId is refused.
	_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "No ID",
	})
	var invalid *SCIMInvalidValueError
	require.ErrorAs(t, err, &invalid)

	// Unassigning the group before SCIM is enforced retires only the binding. Assigning it again binds the same
	// group, which kept its grant.
	require.NoError(t, c.DeleteSCIMGroup(ctx, conn, group.ID))
	rebound, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team",
		ExternalID:  "g-1",
	})
	require.NoError(t, err)
	require.Equal(t, legacyID, rebound.GroupID)
	require.NotEqual(t, group.ID, rebound.ID)
	require.EqualValues(t, 1, rowCount(t, c, new(types.GroupRoleAssignment), "group_name = ?", legacyID))
}

func TestSCIMGroupsByExternalIDWaitForDeletionsAndRefuseOtherProviders(t *testing.T) {
	c := newLookupTestClient(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	tests := []struct {
		name    string
		groupID string
		setup   func(t *testing.T)
	}{
		{
			name:    "a group pending deletion for being unreferenced",
			groupID: lookupTestGroupPrefix + "marked",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
					ID:                    lookupTestGroupPrefix + "marked",
					AuthProviderNamespace: lookupTestProvider.Namespace,
					AuthProviderName:      lookupTestProvider.Name,
					Name:                  "marked",
				}).Error)
				require.NoError(t, c.db.WithContext(ctx).Create(&types.SCIMPendingGroupDeletion{
					GroupID:      lookupTestGroupPrefix + "marked",
					ConnectionID: conn.ID,
					CreatedAt:    time.Now(),
				}).Error)
			},
		},
		{
			name:    "a deleted group whose subjects are still being removed",
			groupID: lookupTestGroupPrefix + "cleanup",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.SCIMGroupSubjectCleanup{
					GroupID:   lookupTestGroupPrefix + "cleanup",
					Namespace: system.DefaultNamespace,
				}).Error)
			},
		},
		{
			name:    "a group of another auth provider",
			groupID: lookupTestGroupPrefix + "other",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
					ID:                    lookupTestGroupPrefix + "other",
					AuthProviderNamespace: system.DefaultNamespace,
					AuthProviderName:      "other-auth-provider",
					Name:                  "other",
				}).Error)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			_, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
				DisplayName: tt.name,
				ExternalID:  strings.TrimPrefix(tt.groupID, lookupTestGroupPrefix),
			})
			var conflict *SCIMConflictError
			require.ErrorAs(t, err, &conflict)
			require.Zero(t, rowCount(t, c, new(types.SCIMGroupBinding), "group_id = ?", tt.groupID))
		})
	}
}

func TestOktaGroupsHaveNoExternalID(t *testing.T) {
	c := newLifecycleTestClient(t)
	conn, _ := createTestSCIMConnection(t, c, false)

	group, err := c.CreateSCIMGroup(t.Context(), conn, SCIMGroupInput{
		DisplayName: "team",
		ExternalID:  "00g1a2b3c4d5e6f7g8h9",
	})
	require.NoError(t, err)
	require.Empty(t, group.ExternalID)

	found, total, err := c.ListSCIMGroups(t.Context(), conn, SCIMGroupFilter{
		ExternalID: "00g1a2b3c4d5e6f7g8h9",
	}, SCIMPage{
		Limit: 10,
	}, false)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, found)
}

// rowCount returns the number of rows of model that match query.
func rowCount(t *testing.T, c *Client, model any, query string, args ...any) int64 {
	t.Helper()

	var n int64
	require.NoError(t, c.db.WithContext(t.Context()).Model(model).Where(query, args...).Count(&n).Error)
	return n
}
