package scim

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/scim/adapter/adaptertest"
)

const (
	lookupProviderName = "lookup-auth-provider"
)

// enableLookupProvider registers an adapter that matches users by group lookup ID, binds groups by externalId, and
// deletes users, and enables SCIM for its auth provider.
func (s *scimTest) enableLookupProvider() {
	s.t.Helper()

	adapter.RegisterForTest(s.t, lookupProviderName, adaptertest.Adapter{
		Users:  adapter.UserKeyGroupLookupID,
		Groups: adapter.GroupKeyExternalID,
		SharedRules: adapter.Rules{
			DeletesUsers: true,
		},
	})
	// SCIM matches the provider's users by group lookup ID only once the backfill of their hashes has run.
	if err := s.gateway.BackfillIdentityGroupLookupIDHashes(s.t.Context()); err != nil {
		s.t.Fatalf("failed to backfill: %v", err)
	}
	s.enableProvider(lookupProviderName, "lookup/", "tenant")
}

func TestUserDeleteDeletesTheUserWhenTheAdapterDeletesUsers(t *testing.T) {
	s := newSCIMTest(t)
	s.enableLookupProvider()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "oid-1")).expect(t, http.StatusCreated)
	binding := s.bindingByID(user.id())

	s.do(http.MethodDelete, "Users/"+user.id(), nil).expect(t, http.StatusNoContent)
	if deleted := s.user(binding.UserID); deleted.DeletedAt == nil {
		t.Fatalf("user %d was not deleted", binding.UserID)
	}

	// The SCIM ID is retired, which the identity provider treats as done.
	s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusNotFound)
	s.do(http.MethodDelete, "Users/"+user.id(), nil).expect(t, http.StatusNotFound)
}

func TestGroupsEchoAndFindTheirExternalID(t *testing.T) {
	s := newSCIMTest(t)
	s.enableLookupProvider()
	s.seedProviderGroup(lookupProviderName, "lookup/g-1", "legacy name")

	body := scimGroup("team")
	body["externalId"] = "g-1"
	created := s.do(http.MethodPost, "Groups", body).expect(t, http.StatusCreated)
	if created.body["externalId"] != "g-1" {
		t.Fatalf("externalId = %v, want g-1", created.body["externalId"])
	}
	if members := s.memberships("lookup/g-1"); len(members) != 0 {
		t.Fatalf("memberships = %v, want none", members)
	}

	found := s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`externalId eq "g-1"`), nil).expect(t, http.StatusOK).resources()
	if len(found) != 1 || found[0]["id"] != created.id() || found[0]["externalId"] != "g-1" {
		t.Fatalf("externalId eq found %v", found)
	}

	// A group without an externalId is refused.
	s.do(http.MethodPost, "Groups", scimGroup("other")).expect(t, http.StatusBadRequest)
}
