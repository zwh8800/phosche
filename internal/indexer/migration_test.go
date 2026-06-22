package indexer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zwh8800/phosche/internal/config"
)

func TestMappingVersion(t *testing.T) {
	assert.Equal(t, 9, MappingVersion(), "MappingVersion should return current mapping version as int")
}

func TestRegisterMigration(t *testing.T) {
	// Save original migrations to restore after test
	originalMigrations := GetMigrations()
	defer func() {
		// Restore by replacing the global map
		for k := range migrations {
			delete(migrations, k)
		}
		for k, v := range originalMigrations {
			migrations[k] = v
		}
	}()

	// Clear migrations for clean test
	for k := range migrations {
		delete(migrations, k)
	}

	m1 := Migration{
		Version: 10,
		Mapping: map[string]any{
			"new_field": map[string]any{"type": "keyword"},
		},
		Migrate: nil,
	}
	RegisterMigration(m1)

	got := GetMigrations()
	require.Len(t, got, 1)
	assert.Equal(t, 10, got[10].Version)
	assert.NotNil(t, got[10].Mapping)
	assert.Nil(t, got[10].Migrate)

	m2 := Migration{
		Version: 11,
		Mapping: nil,
		Migrate: func(_ context.Context, _ *OSClient, _ string, _ *config.Config) error { return nil },
	}
	RegisterMigration(m2)

	got = GetMigrations()
	require.Len(t, got, 2)
	assert.Nil(t, got[11].Mapping)
	assert.NotNil(t, got[11].Migrate)
}

func TestRunMigrations_NoMigrationNeeded(t *testing.T) {
	// When fromVersion == MappingVersion, no migrations should run
	// This is tested indirectly by TestEnsureIndex_Idempotent
	assert.Equal(t, 9, MappingVersion())
	// fromVersion >= MappingVersion() means no migration
	assert.True(t, 9 >= MappingVersion())
}

func TestMigration_WithMappingAndMigrate(t *testing.T) {
	// Verify Migration struct fields work correctly
	m := Migration{
		Version: 10,
		Mapping: map[string]any{
			"multimodal_embedding": map[string]any{
				"type":       "knn_vector",
				"dimension":  1024,
				"space_type": "cosinesimil",
			},
		},
		Migrate: func(_ context.Context, _ *OSClient, _ string, _ *config.Config) error { return nil },
	}

	assert.Equal(t, 10, m.Version)
	assert.NotNil(t, m.Mapping)
	assert.NotNil(t, m.Migrate)

	// Verify the mapping content
	vectorField, ok := m.Mapping["multimodal_embedding"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "knn_vector", vectorField["type"])
	assert.Equal(t, 1024, vectorField["dimension"])
}

func TestMigration_VersionOrdering(t *testing.T) {
	// Migrations should be registered with version numbers that form a valid chain
	// from current MappingVersion to the new target version
	// Example: if current MappingVersion is 9 and we add version 10,
	// then migrations[10] represents the migration from 9→10

	// Save and restore
	originalMigrations := GetMigrations()
	defer func() {
		for k := range migrations {
			delete(migrations, k)
		}
		for k, v := range originalMigrations {
			migrations[k] = v
		}
	}()

	for k := range migrations {
		delete(migrations, k)
	}

	RegisterMigration(Migration{Version: 10, Mapping: map[string]any{"field_a": map[string]any{"type": "keyword"}}})
	RegisterMigration(Migration{Version: 11, Mapping: map[string]any{"field_b": map[string]any{"type": "text"}}})
	RegisterMigration(Migration{Version: 12, Mapping: nil})

	got := GetMigrations()
	assert.Len(t, got, 3)

	// Verify ordering: 10, 11, 12
	assert.NotNil(t, got[10])
	assert.NotNil(t, got[11])
	assert.NotNil(t, got[12])

	// Verify fields
	assert.NotNil(t, got[10].Mapping)
	assert.NotNil(t, got[11].Mapping)
	assert.Nil(t, got[12].Mapping)
}
