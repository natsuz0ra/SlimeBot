package chat

import (
	"testing"

	"slimebot/internal/repositories"
)

func newTestRepo(t *testing.T) *repositories.Repository {
	t.Helper()
	return repositories.New(repositories.NewSQLiteDBTest(t, "chat_services_test"))
}
