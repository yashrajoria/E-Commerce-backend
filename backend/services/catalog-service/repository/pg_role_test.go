package repository

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commondb "github.com/yashrajoria/common/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Needs the catalog_svc role from infrastructure/postgres/catalog_role.sql.
func TestPGRole_CatalogOnlySeesItsOwnSchema(t *testing.T) {
	url := os.Getenv("CATALOG_ROLE_DATABASE_URL")
	if url == "" {
		t.Skip("CATALOG_ROLE_DATABASE_URL not set")
	}
	db, err := gorm.Open(postgres.Open(url), commondb.DefaultGormConfig())
	require.NoError(t, err)

	assert.NoError(t, db.Exec("SELECT 1 FROM catalog.products LIMIT 1").Error, "own schema readable")
	assert.Error(t, db.Exec("SELECT 1 FROM public.users LIMIT 1").Error, "other services' tables denied")
	assert.Error(t, db.Exec("CREATE TABLE catalog.should_fail (id int)").Error, "no DDL")
	assert.Error(t, db.Exec("DROP TABLE catalog.products").Error, "no DROP")
}
