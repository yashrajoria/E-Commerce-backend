package db

import (
	"sort"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ColumnSnapshot maps "table.column" to its data type for the current schema
// (excluding golang-migrate's bookkeeping table).
func ColumnSnapshot(gdb *gorm.DB) (map[string]string, error) {
	rows, err := gdb.Raw(`SELECT table_name || '.' || column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name <> 'schema_migrations'`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snap := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		snap[k] = v
	}
	return snap, rows.Err()
}

// DiffSnapshots lists columns that are new or retyped in after (sorted).
func DiffSnapshots(before, after map[string]string) []string {
	var out []string
	for k, v := range after {
		if old, ok := before[k]; !ok {
			out = append(out, "added "+k+" ("+v+")")
		} else if old != v {
			out = append(out, "retyped "+k+" ("+old+" -> "+v+")")
		}
	}
	sort.Strings(out)
	return out
}

// AutoMigrateDrift runs AutoMigrate for models against the database at url and
// returns the columns it had to add or retype. Run it on a database built only
// by the SQL migrations: a non-empty result means the migrations and the Go
// models disagree. It mutates that database.
func AutoMigrateDrift(url string, models ...interface{}) ([]string, error) {
	gdb, err := gorm.Open(postgres.Open(url), DefaultGormConfig())
	if err != nil {
		return nil, err
	}
	before, err := ColumnSnapshot(gdb)
	if err != nil {
		return nil, err
	}
	if err := gdb.AutoMigrate(models...); err != nil {
		return nil, err
	}
	after, err := ColumnSnapshot(gdb)
	if err != nil {
		return nil, err
	}
	return DiffSnapshots(before, after), nil
}
