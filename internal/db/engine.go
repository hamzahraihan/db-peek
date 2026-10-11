package db

import (
	"context"
)

// Engine abstracts non-SQL or specialized storage backends (e.g. Redis, MongoDB)
// so DB can route calls transparently without changing caller signatures.
type Engine interface {
	Ping(ctx context.Context) error
	Close() error
	ListSchemas(ctx context.Context) ([]string, error)
	ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error)
	Columns(ctx context.Context, q QualTable) ([]Column, error)
	PrimaryKey(ctx context.Context, q QualTable) ([]string, error)
	Indexes(ctx context.Context, q QualTable) ([]Index, error)
	ForeignKeys(ctx context.Context, q QualTable) ([]ForeignKey, error)
	Count(ctx context.Context, q QualTable) (int64, error)
	PageRows(ctx context.Context, q QualTable, limit, offset int) (*Sample, error)
	Query(ctx context.Context, q string) (*Sample, error)
	RunUserQuery(ctx context.Context, q string) (*Sample, int64, error)
	ExecStmt(ctx context.Context, q string) (int64, error)
	UpdateCell(ctx context.Context, q QualTable, pk []string, pkVals []any, col string, val any) (int64, error)
	TableDDL(ctx context.Context, q QualTable) (string, error)
	Explain(q string, analyze bool) string
}
