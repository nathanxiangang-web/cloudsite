package migrations

import (
	"strings"
	"testing"
)

func TestPrivateCloudCopyMigrationScope(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	migration := all[len(all)-1]
	if migration.Version != 22 || migration.Filename != "0022_private_cloud_copy.sql" {
		t.Fatalf("unexpected migration: %#v", migration)
	}
	sql := strings.ToLower(migration.SQL)
	for _, required := range []string{
		"create table cloud_user_spaces", "alter table acquisition_manifests",
		"add column user_space_id", "add column target_provider_scope",
		"create table acquisition_result_object_evidence", "create table acquisition_copy_operations",
		"destination_before jsonb", "'copy_reserved'", "'provider_unknown'",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"update ", "delete from", "insert into", "operation_limit", "concurrency_limit",
		"user_capacity", "user_quota", "520g", "520gb", "drop table",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains forbidden %q", forbidden)
		}
	}
}

func TestPrivateCloudCopyMigrationLeavesHistoricalManifestsNullable(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(all[len(all)-1].SQL)
	for _, required := range []string{
		"user_space_id uuid null", "target_provider_connection_id uuid null", "target_provider_scope text null",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("missing nullable historical field %q", required)
		}
	}
	if strings.Contains(sql, "set default") || strings.Contains(sql, " backfill") {
		t.Fatal("private target must not receive a historical backfill/default")
	}
}
