package types

// ImportState encodes an import round counter.
type ImportState struct {
	NextRoundToAccount uint64 `codec:"next_account_round"`
}

// MigrationState is metadata used by the postgres migrations.
type MigrationState struct {
	NextMigration int `json:"next"`
}
