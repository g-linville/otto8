package adapter

import (
	"strings"
	"testing"
)

const (
	// testAdapterType is the persisted type of TestAdapter.
	testAdapterType = "test"
)

// TestAdapter is an adapter for tests of shared code that need rules no registered adapter has. Its rules are Okta's,
// apart from those its fields choose. With UserKeyGroupLookupID, a user that SCIM creates has a pending identity, and
// with GroupKeyExternalID, a group's ID is the prefix and its externalId.
type TestAdapter struct {
	Users  UserKey
	Groups GroupKey
	Shared Rules
}

// RegisterForTest registers a for the named auth provider until the test ends. Tests that register an adapter must not
// run in parallel with tests that look adapters up.
func RegisterForTest(tb testing.TB, authProviderName string, a TestAdapter) {
	tb.Helper()

	byAuthProvider[authProviderName] = a
	byType[testAdapterType] = a
	tb.Cleanup(func() {
		delete(byAuthProvider, authProviderName)
		delete(byType, testAdapterType)
	})
}

func (TestAdapter) Type() string {
	return testAdapterType
}

func (TestAdapter) DirectoryParameters() []DirectoryParameter {
	return okta{}.DirectoryParameters()
}

func (TestAdapter) SCIMChoiceParameter() SCIMChoiceParameter {
	return SCIMChoiceParameter{}
}

func (TestAdapter) IssuerConfigurationParameter() string {
	return okta{}.IssuerConfigurationParameter()
}

func (TestAdapter) NativeUserID(user User) (string, error) {
	return okta{}.NativeUserID(user)
}

func (a TestAdapter) UserKey() UserKey {
	return a.Users
}

func (a TestAdapter) NewUserIdentity(nativeUserID string) Identity {
	if a.Users != UserKeyGroupLookupID {
		return okta{}.NewUserIdentity(nativeUserID)
	}
	return Identity{
		Username:              nativeUserID,
		ProviderUsername:      PendingProviderUserID(nativeUserID),
		ProviderUserID:        PendingProviderUserID(nativeUserID),
		ProviderGroupLookupID: nativeUserID,
	}
}

func (a TestAdapter) GroupKey() GroupKey {
	return a.Groups
}

func (a TestAdapter) NativeGroupIDFromExternalID(externalID string) (string, error) {
	id := strings.TrimSpace(externalID)
	if a.Groups != GroupKeyExternalID || id == "" {
		return "", &InvalidGroupError{
			Message: "externalId must be the group's native ID",
		}
	}
	return id, nil
}

func (a TestAdapter) NewGroupID(groupIDPrefix, scimID, nativeGroupID string) string {
	if a.Groups == GroupKeyExternalID {
		return groupIDPrefix + nativeGroupID
	}
	return groupIDPrefix + scimID
}

func (a TestAdapter) NativeGroupID(groupIDPrefix, groupID string) string {
	if a.Groups != GroupKeyExternalID {
		return ""
	}
	if id, ok := strings.CutPrefix(groupID, groupIDPrefix); ok && groupIDPrefix != "" {
		return id
	}
	return ""
}

func (TestAdapter) GroupConsoleURL(string, string) string {
	return ""
}

func (TestAdapter) PatchRules() PatchRules {
	return PatchRules{}
}

func (a TestAdapter) Rules() Rules {
	return a.Shared
}

func (TestAdapter) ErrorHints() ErrorHints {
	return ErrorHints{}
}
