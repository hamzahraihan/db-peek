package db

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoEngine struct {
	client    *mongo.Client
	defaultDB string
	rawURL    string
}

func openMongo(raw string) (Engine, string, error) {
	opts := options.Client().ApplyURI(raw)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, "", fmt.Errorf("mongo connection error: %w", err)
	}
	dbName := "test"
	if u, err := url.Parse(raw); err == nil && u.Path != "" && u.Path != "/" {
		dbName = strings.TrimPrefix(u.Path, "/")
	}
	return &mongoEngine{
		client:    client,
		defaultDB: dbName,
		rawURL:    raw,
	}, dbName, nil
}

func (m *mongoEngine) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, nil)
}

func (m *mongoEngine) Close() error {
	return m.client.Disconnect(ctxBackground())
}

func ctxBackground() context.Context {
	return context.Background()
}

func (m *mongoEngine) ListSchemas(ctx context.Context) ([]string, error) {
	dbs, err := m.client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return []string{m.defaultDB}, nil
	}
	var out []string
	hasDefault := false
	for _, name := range dbs {
		if name == "admin" || name == "config" || name == "local" {
			continue
		}
		if name == m.defaultDB {
			hasDefault = true
		} else {
			out = append(out, name)
		}
	}
	if hasDefault || len(out) == 0 {
		out = append([]string{m.defaultDB}, out...)
	}
	return out, nil
}

func (m *mongoEngine) getDB(schema string) *mongo.Database {
	if schema == "" {
		schema = m.defaultDB
	}
	return m.client.Database(schema)
}

func (m *mongoEngine) ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error) {
	db := m.getDB(schema)
	names, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	out := make([]TableRef, len(names))
	for i, n := range names {
		out[i] = TableRef{Name: n, IsView: false}
	}
	return out, nil
}

func (m *mongoEngine) Columns(ctx context.Context, q QualTable) ([]Column, error) {
	db := m.getDB(q.Schema)
	coll := db.Collection(q.Name)

	cur, err := coll.Find(ctx, bson.M{}, options.Find().SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	typeMap := make(map[string]string)
	typeMap["_id"] = "objectId"

	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		for k, v := range doc {
			if _, exists := typeMap[k]; !exists {
				typeMap[k] = inferBSONType(v)
			}
		}
	}

	var names []string
	for k := range typeMap {
		if k != "_id" {
			names = append(names, k)
		}
	}
	sort.Strings(names)

	cols := []Column{
		{Name: "_id", Type: typeMap["_id"], Nullable: "NO", Extra: "PK"},
	}
	for _, n := range names {
		cols = append(cols, Column{
			Name:     n,
			Type:     typeMap[n],
			Nullable: "YES",
		})
	}
	return cols, nil
}

func inferBSONType(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case int, int32, int64:
		return "integer"
	case float32, float64:
		return "double"
	case bool:
		return "boolean"
	case bson.M, map[string]any:
		return "document"
	case bson.A, []any:
		return "array"
	case bson.ObjectID:
		return "objectId"
	case bson.DateTime:
		return "date"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func (m *mongoEngine) PrimaryKey(ctx context.Context, q QualTable) ([]string, error) {
	return []string{"_id"}, nil
}

func (m *mongoEngine) Indexes(ctx context.Context, q QualTable) ([]Index, error) {
	db := m.getDB(q.Schema)
	coll := db.Collection(q.Name)

	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		return nil, nil
	}
	defer cur.Close(ctx)

	var out []Index
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		name, _ := doc["name"].(string)
		unique := "no"
		if u, ok := doc["unique"].(bool); ok && u {
			unique = "yes"
		}
		keyBytes, _ := json.Marshal(doc["key"])
		out = append(out, Index{
			Name:    name,
			Unique:  unique,
			Columns: string(keyBytes),
		})
	}
	return out, nil
}

func (m *mongoEngine) ForeignKeys(ctx context.Context, q QualTable) ([]ForeignKey, error) {
	return nil, nil
}

func (m *mongoEngine) Count(ctx context.Context, q QualTable) (int64, error) {
	if m.client == nil {
		return 0, nil
	}
	db := m.getDB(q.Schema)
	coll := db.Collection(q.Name)
	return coll.EstimatedDocumentCount(ctx)
}

func (m *mongoEngine) PageRows(ctx context.Context, q QualTable, limit, offset int) (*Sample, error) {
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	db := m.getDB(q.Schema)
	coll := db.Collection(q.Name)

	cur, err := coll.Find(ctx, bson.M{}, options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var docs []bson.M
	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}

	cols, err := m.Columns(ctx, q)
	if err != nil {
		return nil, err
	}
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.Name
	}

	sample := &Sample{Columns: colNames, Total: -1}
	for _, doc := range docs {
		rec := make([]string, len(colNames))
		raw := make([]any, len(colNames))
		for i, col := range colNames {
			val := doc[col]
			raw[i] = val
			rec[i] = formatBSONValue(val)
		}
		sample.Rows = append(sample.Rows, rec)
		sample.Raw = append(sample.Raw, raw)
	}
	return sample, nil
}

func formatBSONValue(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return truncate(val, 60)
	case bson.ObjectID:
		return val.Hex()
	case bson.M, map[string]any, bson.A, []any:
		b, err := json.Marshal(val)
		if err == nil {
			return truncate(string(b), 60)
		}
		return truncate(fmt.Sprintf("%v", val), 60)
	default:
		return truncate(fmt.Sprintf("%v", val), 60)
	}
}

func (m *mongoEngine) Query(ctx context.Context, q string) (*Sample, error) {
	s, _, err := m.RunUserQuery(ctx, q)
	return s, err
}

func (m *mongoEngine) RunUserQuery(ctx context.Context, q string) (*Sample, int64, error) {
	q = strings.TrimSpace(q)
	for strings.HasSuffix(q, ";") {
		q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	}
	if q == "" {
		return nil, 0, nil
	}

	upper := strings.ToUpper(q)
	if strings.HasPrefix(upper, "SELECT") {
		collName := "test"
		words := strings.Fields(q)
		for i, w := range words {
			if strings.EqualFold(w, "FROM") && i+1 < len(words) {
				collName = strings.Trim(words[i+1], "\"`[]'")
				break
			}
		}
		s, err := m.PageRows(ctx, QualTable{Name: collName}, 200, 0)
		return s, -1, err
	}

	// Try parsing as JSON MongoDB command or filter.
	var doc bson.M
	if err := json.Unmarshal([]byte(q), &doc); err == nil {
		// If query has "find" key: {find: "collection", filter: {...}}
		if collVal, ok := doc["find"].(string); ok {
			filter := bson.M{}
			if f, ok := doc["filter"].(map[string]any); ok {
				filter = f
			}
			db := m.getDB("")
			cur, err := db.Collection(collVal).Find(ctx, filter, options.Find().SetLimit(200))
			if err != nil {
				return nil, 0, err
			}
			defer cur.Close(ctx)
			return m.cursorToSample(ctx, cur, collVal)
		}

		// Run raw database command.
		db := m.getDB("")
		var res bson.M
		err := db.RunCommand(ctx, doc).Decode(&res)
		if err != nil {
			return nil, 0, err
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return &Sample{
			Columns: []string{"command_result"},
			Rows:    [][]string{{truncate(string(b), 120)}},
			Raw:     [][]any{{res}},
			Total:   -1,
		}, -1, nil
	}

	// Try db.<collection>.find(...) syntax
	if strings.HasPrefix(q, "db.") {
		parts := strings.SplitN(strings.TrimPrefix(q, "db."), ".", 2)
		if len(parts) == 2 {
			collName := parts[0]
			rest := parts[1]
			if strings.HasPrefix(rest, "find(") {
				inside := strings.TrimSuffix(strings.TrimPrefix(rest, "find("), ")")
				filter := bson.M{}
				if strings.TrimSpace(inside) != "" {
					_ = json.Unmarshal([]byte(inside), &filter)
				}
				cur, err := m.getDB("").Collection(collName).Find(ctx, filter, options.Find().SetLimit(200))
				if err != nil {
					return nil, 0, err
				}
				defer cur.Close(ctx)
				return m.cursorToSample(ctx, cur, collName)
			}
		}
	}

	return nil, 0, fmt.Errorf("unsupported mongodb query syntax (use SELECT ... FROM <collection>, {find: \"coll\"}, or JSON command)")
}

func (m *mongoEngine) cursorToSample(ctx context.Context, cur *mongo.Cursor, coll string) (*Sample, int64, error) {
	var docs []bson.M
	keySet := make(map[string]bool)
	keySet["_id"] = true

	for cur.Next(ctx) {
		var doc bson.M
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}
		docs = append(docs, doc)
		for k := range doc {
			keySet[k] = true
		}
	}

	var names []string
	for k := range keySet {
		if k != "_id" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	cols := append([]string{"_id"}, names...)

	sample := &Sample{Columns: cols, Total: -1}
	for _, doc := range docs {
		rec := make([]string, len(cols))
		raw := make([]any, len(cols))
		for i, c := range cols {
			val := doc[c]
			raw[i] = val
			rec[i] = formatBSONValue(val)
		}
		sample.Rows = append(sample.Rows, rec)
		sample.Raw = append(sample.Raw, raw)
	}
	return sample, -1, nil
}

func (m *mongoEngine) ExecStmt(ctx context.Context, q string) (int64, error) {
	_, n, err := m.RunUserQuery(ctx, q)
	return n, err
}

func (m *mongoEngine) UpdateCell(ctx context.Context, q QualTable, pk []string, pkVals []any, col string, val any) (int64, error) {
	if len(pkVals) == 0 {
		return 0, fmt.Errorf("mongo update: _id required")
	}
	idVal := pkVals[0]
	// If id is string representation of ObjectID, convert back
	if idStr, ok := idVal.(string); ok {
		if oid, err := bson.ObjectIDFromHex(idStr); err == nil {
			idVal = oid
		}
	}

	db := m.getDB(q.Schema)
	coll := db.Collection(q.Name)
	res, err := coll.UpdateOne(ctx, bson.M{"_id": idVal}, bson.M{"$set": bson.M{col: val}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

func (m *mongoEngine) TableDDL(ctx context.Context, q QualTable) (string, error) {
	count, _ := m.Count(ctx, q)
	return fmt.Sprintf("# MongoDB Collection: %s.%s\n# Estimated Documents: %d\n# Primary Key: _id (ObjectId)", q.Schema, q.Name, count), nil
}

func (m *mongoEngine) Explain(q string, analyze bool) string {
	return fmt.Sprintf("{explain: %s}", q)
}
