package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devdash.db")

	db, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow(`
		SELECT MAX(version_id)
		FROM goose_db_version
		WHERE is_applied = 1
	`).Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if version != 4 {
		t.Fatalf("migration version = %d, want 4", version)
	}

	var resourceTypes, relationTypes int
	if err := db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM resource_types WHERE name = 'repository'),
			(SELECT COUNT(*) FROM relation_types WHERE name = 'related_to')
	`).Scan(&resourceTypes, &relationTypes); err != nil {
		t.Fatalf("query seeded metadata: %v", err)
	}
	if resourceTypes != 1 {
		t.Errorf("repository resource type count = %d, want 1", resourceTypes)
	}
	if relationTypes != 1 {
		t.Errorf("related_to relation type count = %d, want 1", relationTypes)
	}

	var secretColumns int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('secrets')
		WHERE name IN ('key', 'value', 'description', 'created_at', 'updated_at')
	`).Scan(&secretColumns); err != nil {
		t.Fatalf("query secrets schema: %v", err)
	}
	if secretColumns != 5 {
		t.Errorf("secrets column count = %d, want 5", secretColumns)
	}

	type columnInfo struct {
		name         string
		columnType   string
		defaultValue string
		notNull      int
		pk           int
	}
	columnRows, err := db.Query(`
		SELECT name, "type", COALESCE(dflt_value, ''), "notnull", pk
		FROM pragma_table_info('workspace_config')
		ORDER BY cid
	`)
	if err != nil {
		t.Fatalf("query workspace_config columns: %v", err)
	}
	defer columnRows.Close()

	var columns []columnInfo
	for columnRows.Next() {
		var column columnInfo
		if err := columnRows.Scan(&column.name, &column.columnType, &column.defaultValue, &column.notNull, &column.pk); err != nil {
			t.Fatalf("scan workspace_config column: %v", err)
		}
		columns = append(columns, column)
	}
	if err := columnRows.Err(); err != nil {
		t.Fatalf("iterate workspace_config columns: %v", err)
	}
	wantColumns := []columnInfo{
		{name: "workspace_id", columnType: "TEXT", notNull: 1, pk: 1},
		{name: "namespace", columnType: "TEXT", notNull: 1, pk: 2},
		{name: "key", columnType: "TEXT", notNull: 1, pk: 3},
		{name: "value", columnType: "TEXT", notNull: 1},
		{name: "created_at", columnType: "DATETIME", defaultValue: "CURRENT_TIMESTAMP", notNull: 1},
		{name: "updated_at", columnType: "DATETIME", defaultValue: "CURRENT_TIMESTAMP", notNull: 1},
	}
	if len(columns) != len(wantColumns) {
		t.Fatalf("workspace_config columns = %#v, want %#v", columns, wantColumns)
	}
	for i := range wantColumns {
		if columns[i] != wantColumns[i] {
			t.Errorf("workspace_config column %d = %#v, want %#v", i, columns[i], wantColumns[i])
		}
	}

	var foreignTable, foreignFrom, foreignTo, onDelete string
	if err := db.QueryRow(`
		SELECT "table", "from", "to", on_delete
		FROM pragma_foreign_key_list('workspace_config')
		WHERE "from" = 'workspace_id'
	`).Scan(&foreignTable, &foreignFrom, &foreignTo, &onDelete); err != nil {
		t.Fatalf("query workspace_config foreign key: %v", err)
	}
	if foreignTable != "workspaces" || foreignFrom != "workspace_id" || foreignTo != "id" || onDelete != "CASCADE" {
		t.Errorf("workspace_config foreign key = (%q, %q, %q, %q), want (workspaces, workspace_id, id, CASCADE)", foreignTable, foreignFrom, foreignTo, onDelete)
	}

	var indexUnique, indexPartial int
	if err := db.QueryRow(`
		SELECT "unique", partial
		FROM pragma_index_list('workspace_config')
		WHERE name = 'idx_workspace_config_namespace'
	`).Scan(&indexUnique, &indexPartial); err != nil {
		t.Fatalf("query workspace_config namespace index: %v", err)
	}
	if indexUnique != 0 || indexPartial != 0 {
		t.Errorf("namespace index flags = (unique=%d, partial=%d), want (unique=0, partial=0)", indexUnique, indexPartial)
	}

	indexRows, err := db.Query(`
		SELECT name
		FROM pragma_index_info('idx_workspace_config_namespace')
		ORDER BY seqno
	`)
	if err != nil {
		t.Fatalf("query namespace index columns: %v", err)
	}
	defer indexRows.Close()
	var indexColumns []string
	for indexRows.Next() {
		var name string
		if err := indexRows.Scan(&name); err != nil {
			t.Fatalf("scan namespace index column: %v", err)
		}
		indexColumns = append(indexColumns, name)
	}
	if err := indexRows.Err(); err != nil {
		t.Fatalf("iterate namespace index columns: %v", err)
	}
	if len(indexColumns) != 2 || indexColumns[0] != "workspace_id" || indexColumns[1] != "namespace" {
		t.Errorf("namespace index columns = %v, want [workspace_id namespace]", indexColumns)
	}
}
