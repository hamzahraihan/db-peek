package db

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestQuoteIdentAllEngines(t *testing.T) {
	cases := []struct {
		driver Driver
		ident  string
		want   string
	}{
		{Postgres, "users", `"users"`},
		{Postgres, `weird"name`, `"weird""name"`},
		{SQLite, "users", `"users"`},
		{MySQL, "users", "`users`"},
		{MySQL, "weird`name", "`weird``name`"},
		{MariaDB, "users", "`users`"},
		{MariaDB, "weird`name", "`weird``name`"},
		{ClickHouse, "users", "`users`"},
		{ClickHouse, "weird`name", "`weird``name`"},
		{SQLServer, "users", "[users]"},
		{SQLServer, "weird]name", "[weird]]name]"},
	}

	for _, c := range cases {
		if got := c.driver.QuoteIdent(c.ident); got != c.want {
			t.Errorf("%s.QuoteIdent(%q) = %q, want %q", c.driver, c.ident, got, c.want)
		}
	}
}

func TestQuoteQualAllEngines(t *testing.T) {
	cases := []struct {
		driver Driver
		qual   QualTable
		want   string
	}{
		{Postgres, QualTable{Schema: "public", Name: "users"}, `"public"."users"`},
		{Postgres, QualTable{Name: "users"}, `"users"`},
		{MySQL, QualTable{Schema: "mydb", Name: "users"}, "`mydb`.`users`"},
		{MariaDB, QualTable{Schema: "mydb", Name: "users"}, "`mydb`.`users`"},
		{ClickHouse, QualTable{Schema: "default", Name: "hits"}, "`default`.`hits`"},
		{SQLServer, QualTable{Schema: "dbo", Name: "users"}, "[dbo].[users]"},
		{SQLServer, QualTable{Name: "users"}, "[users]"},
	}

	for _, c := range cases {
		if got := c.driver.QuoteQual(c.qual); got != c.want {
			t.Errorf("%s.QuoteQual(%+v) = %q, want %q", c.driver, c.qual, got, c.want)
		}
	}
}

func TestPlaceholdersAllEngines(t *testing.T) {
	if got := Postgres.Placeholder(1); got != "$1" {
		t.Errorf("postgres placeholder 1 = %q, want $1", got)
	}
	if got := Postgres.Placeholder(5); got != "$5" {
		t.Errorf("postgres placeholder 5 = %q, want $5", got)
	}

	if got := SQLServer.Placeholder(1); got != "@p1" {
		t.Errorf("sqlserver placeholder 1 = %q, want @p1", got)
	}
	if got := SQLServer.Placeholder(3); got != "@p3" {
		t.Errorf("sqlserver placeholder 3 = %q, want @p3", got)
	}

	for _, d := range []Driver{MySQL, MariaDB, SQLite, ClickHouse} {
		for i := 1; i <= 3; i++ {
			if got := d.Placeholder(i); got != "?" {
				t.Errorf("%s placeholder %d = %q, want ?", d, i, got)
			}
		}
	}
}

func TestSplitRedisArgs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"GET foo", []string{"GET", "foo"}},
		{"SET foo \"hello world\"", []string{"SET", "foo", "hello world"}},
		{"HSET myhash 'field 1' 'value 1'", []string{"HSET", "myhash", "field 1", "value 1"}},
		{"MSET k1 v1 k2 v2", []string{"MSET", "k1", "v1", "k2", "v2"}},
		{"   DEL   key1   key2   ", []string{"DEL", "key1", "key2"}},
	}

	for _, c := range cases {
		got := splitRedisArgs(c.input)
		if len(got) != len(c.want) {
			t.Fatalf("splitRedisArgs(%q) len = %d, want %d (%v)", c.input, len(got), len(c.want), got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitRedisArgs(%q)[%d] = %q, want %q", c.input, i, got[i], c.want[i])
			}
		}
	}
}

func TestRedisEngineVirtualMetadata(t *testing.T) {
	eng := &redisEngine{dbNum: 0}

	ctx := context.Background()
	cols, err := eng.Columns(ctx, QualTable{Name: "hashes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 || cols[0].Name != "key" || cols[1].Name != "field" || cols[2].Name != "value" {
		t.Errorf("unexpected hashes columns: %+v", cols)
	}

	pk, err := eng.PrimaryKey(ctx, QualTable{Name: "hashes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pk) != 1 || pk[0] != "key" {
		t.Errorf("unexpected hashes PK: %+v", pk)
	}

	indexes, err := eng.Indexes(ctx, QualTable{Name: "hashes"})
	if err != nil || indexes != nil {
		t.Errorf("expected nil indexes for redis: %+v", indexes)
	}

	fks, err := eng.ForeignKeys(ctx, QualTable{Name: "hashes"})
	if err != nil || fks != nil {
		t.Errorf("expected nil foreign keys for redis: %+v", fks)
	}

	exp := eng.Explain("GET mykey", false)
	if !strings.Contains(exp, "MEMORY USAGE mykey") {
		t.Errorf("unexpected redis explain: %q", exp)
	}
}

func TestMongoEngineVirtualMetadata(t *testing.T) {
	eng := &mongoEngine{defaultDB: "test"}

	ctx := context.Background()
	pk, err := eng.PrimaryKey(ctx, QualTable{Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pk) != 1 || pk[0] != "_id" {
		t.Errorf("unexpected mongo PK: %+v", pk)
	}

	fks, err := eng.ForeignKeys(ctx, QualTable{Name: "users"})
	if err != nil || fks != nil {
		t.Errorf("expected nil foreign keys for mongo: %+v", fks)
	}

	ddl, err := eng.TableDDL(ctx, QualTable{Schema: "test", Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "MongoDB Collection: test.users") {
		t.Errorf("unexpected mongo DDL: %q", ddl)
	}

	exp := eng.Explain("{find: 'users'}", false)
	if !strings.Contains(exp, "{explain: {find: 'users'}}") {
		t.Errorf("unexpected mongo explain: %q", exp)
	}
}

func TestInferBSONType(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{nil, "null"},
		{"hello", "string"},
		{int32(42), "integer"},
		{int64(42), "integer"},
		{3.14, "double"},
		{true, "boolean"},
		{bson.ObjectID{}, "objectId"},
		{bson.M{"a": 1}, "document"},
		{bson.A{1, 2}, "array"},
	}

	for _, c := range cases {
		if got := inferBSONType(c.val); got != c.want {
			t.Errorf("inferBSONType(%v) = %q, want %q", c.val, got, c.want)
		}
	}
}

func TestFormatBSONValue(t *testing.T) {
	if got := formatBSONValue(nil); got != "NULL" {
		t.Errorf("formatBSONValue(nil) = %q, want NULL", got)
	}
	if got := formatBSONValue("hello"); got != "hello" {
		t.Errorf("formatBSONValue(string) = %q, want hello", got)
	}
	oid := bson.ObjectID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if got := formatBSONValue(oid); got != oid.Hex() {
		t.Errorf("formatBSONValue(oid) = %q, want %q", got, oid.Hex())
	}
}
