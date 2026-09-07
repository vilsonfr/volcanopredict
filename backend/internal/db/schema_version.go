package db

// ExpectedSchemaVersion is the migration version this binary was built
// against. /health refuses to report healthy when the database is behind it,
// so a partially-deployed rollout fails loudly instead of answering queries
// against columns that do not exist yet.
//
// Keep this in step with the highest-numbered file in migrations/.
const ExpectedSchemaVersion int64 = 10
