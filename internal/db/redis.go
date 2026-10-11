package db

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type redisEngine struct {
	client *redis.Client
	dbNum  int
	rawURL string
}

func openRedis(raw string) (Engine, string, error) {
	opt, err := redis.ParseURL(raw)
	if err != nil {
		return nil, "", fmt.Errorf("redis connection error: %w", err)
	}
	rdb := redis.NewClient(opt)
	name := fmt.Sprintf("db%d", opt.DB)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		name = fmt.Sprintf("%s/db%d", u.Host, opt.DB)
	}
	return &redisEngine{
		client: rdb,
		dbNum:  opt.DB,
		rawURL: raw,
	}, name, nil
}

func (r *redisEngine) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *redisEngine) Close() error {
	return r.client.Close()
}

func (r *redisEngine) ListSchemas(ctx context.Context) ([]string, error) {
	current := fmt.Sprintf("db%d", r.dbNum)
	schemas := []string{current}
	info, err := r.client.Info(ctx, "keyspace").Result()
	if err == nil {
		for _, line := range strings.Split(info, "\r\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "db") {
				parts := strings.Split(line, ":")
				dbName := parts[0]
				if dbName != current {
					schemas = append(schemas, dbName)
				}
			}
		}
	}
	if len(schemas) == 1 {
		for i := range 16 {
			name := fmt.Sprintf("db%d", i)
			if name != current {
				schemas = append(schemas, name)
			}
		}
	}
	return schemas, nil
}

func (r *redisEngine) ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error) {
	base := []TableRef{
		{Name: "keys", IsView: false},
		{Name: "strings", IsView: false},
		{Name: "hashes", IsView: false},
		{Name: "lists", IsView: false},
		{Name: "sets", IsView: false},
		{Name: "zsets", IsView: false},
	}
	// Sample keys to find common prefix namespaces (e.g. user:123 -> user).
	keys, _, err := r.client.Scan(ctx, 0, "*", 100).Result()
	if err == nil {
		seen := make(map[string]bool)
		for _, k := range keys {
			if idx := strings.Index(k, ":"); idx > 0 {
				prefix := k[:idx]
				if !seen[prefix] {
					seen[prefix] = true
					base = append(base, TableRef{Name: prefix, IsView: true})
				}
			}
		}
	}
	return base, nil
}

func (r *redisEngine) Columns(ctx context.Context, q QualTable) ([]Column, error) {
	switch q.Name {
	case "hashes":
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "field", Type: "string", Nullable: "NO"},
			{Name: "value", Type: "string", Nullable: "YES"},
		}, nil
	case "lists":
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "index", Type: "integer", Nullable: "NO"},
			{Name: "value", Type: "string", Nullable: "YES"},
		}, nil
	case "sets":
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "member", Type: "string", Nullable: "NO"},
		}, nil
	case "zsets":
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "member", Type: "string", Nullable: "NO"},
			{Name: "score", Type: "float", Nullable: "NO"},
		}, nil
	case "strings":
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "value", Type: "string", Nullable: "YES"},
			{Name: "ttl_seconds", Type: "integer", Nullable: "YES"},
		}, nil
	default:
		return []Column{
			{Name: "key", Type: "string", Nullable: "NO", Extra: "PK"},
			{Name: "type", Type: "string", Nullable: "NO"},
			{Name: "ttl_seconds", Type: "integer", Nullable: "YES"},
			{Name: "value", Type: "string", Nullable: "YES"},
		}, nil
	}
}

func (r *redisEngine) PrimaryKey(ctx context.Context, q QualTable) ([]string, error) {
	return []string{"key"}, nil
}

func (r *redisEngine) Indexes(ctx context.Context, q QualTable) ([]Index, error) {
	return nil, nil
}

func (r *redisEngine) ForeignKeys(ctx context.Context, q QualTable) ([]ForeignKey, error) {
	return nil, nil
}

func (r *redisEngine) Count(ctx context.Context, q QualTable) (int64, error) {
	if r.client == nil {
		return 0, nil
	}
	return r.client.DBSize(ctx).Result()
}

func (r *redisEngine) PageRows(ctx context.Context, q QualTable, limit, offset int) (*Sample, error) {
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	pattern := "*"
	if q.Name != "" && q.Name != "keys" && q.Name != "strings" && q.Name != "hashes" && q.Name != "lists" && q.Name != "sets" && q.Name != "zsets" {
		pattern = q.Name + ":*"
	}

	totalNeeded := offset + limit
	var allKeys []string
	var cursor uint64
	for {
		var batch []string
		var err error
		batch, cursor, err = r.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		allKeys = append(allKeys, batch...)
		if len(allKeys) >= totalNeeded || cursor == 0 {
			break
		}
	}

	cols, err := r.Columns(ctx, q)
	if err != nil {
		return nil, err
	}
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.Name
	}
	sample := &Sample{Columns: colNames, Total: -1}

	start := offset
	if start > len(allKeys) {
		start = len(allKeys)
	}
	end := start + limit
	if end > len(allKeys) {
		end = len(allKeys)
	}
	sliceKeys := allKeys[start:end]

	switch q.Name {
	case "hashes":
		for _, k := range sliceKeys {
			vals, err := r.client.HGetAll(ctx, k).Result()
			if err != nil {
				continue
			}
			for f, v := range vals {
				sample.Rows = append(sample.Rows, []string{truncate(k, 60), truncate(f, 60), truncate(v, 60)})
				sample.Raw = append(sample.Raw, []any{k, f, v})
				if len(sample.Rows) >= limit {
					break
				}
			}
			if len(sample.Rows) >= limit {
				break
			}
		}
	case "lists":
		for _, k := range sliceKeys {
			vals, err := r.client.LRange(ctx, k, 0, int64(limit-1)).Result()
			if err != nil {
				continue
			}
			for idx, v := range vals {
				sample.Rows = append(sample.Rows, []string{truncate(k, 60), strconv.Itoa(idx), truncate(v, 60)})
				sample.Raw = append(sample.Raw, []any{k, int64(idx), v})
				if len(sample.Rows) >= limit {
					break
				}
			}
			if len(sample.Rows) >= limit {
				break
			}
		}
	case "sets":
		for _, k := range sliceKeys {
			vals, err := r.client.SMembers(ctx, k).Result()
			if err != nil {
				continue
			}
			for _, m := range vals {
				sample.Rows = append(sample.Rows, []string{truncate(k, 60), truncate(m, 60)})
				sample.Raw = append(sample.Raw, []any{k, m})
				if len(sample.Rows) >= limit {
					break
				}
			}
			if len(sample.Rows) >= limit {
				break
			}
		}
	case "zsets":
		for _, k := range sliceKeys {
			vals, err := r.client.ZRangeWithScores(ctx, k, 0, int64(limit-1)).Result()
			if err != nil {
				continue
			}
			for _, z := range vals {
				mStr := fmt.Sprintf("%v", z.Member)
				sStr := fmt.Sprintf("%v", z.Score)
				sample.Rows = append(sample.Rows, []string{truncate(k, 60), truncate(mStr, 60), sStr})
				sample.Raw = append(sample.Raw, []any{k, z.Member, z.Score})
				if len(sample.Rows) >= limit {
					break
				}
			}
			if len(sample.Rows) >= limit {
				break
			}
		}
	case "strings":
		for _, k := range sliceKeys {
			kType, _ := r.client.Type(ctx, k).Result()
			if kType != "string" {
				continue
			}
			val, _ := r.client.Get(ctx, k).Result()
			ttl, _ := r.client.TTL(ctx, k).Result()
			ttlSec := int64(ttl.Seconds())
			sample.Rows = append(sample.Rows, []string{truncate(k, 60), truncate(val, 60), strconv.FormatInt(ttlSec, 10)})
			sample.Raw = append(sample.Raw, []any{k, val, ttlSec})
		}
	default: // "keys" or prefix table
		for _, k := range sliceKeys {
			kType, _ := r.client.Type(ctx, k).Result()
			ttl, _ := r.client.TTL(ctx, k).Result()
			ttlSec := int64(ttl.Seconds())
			var valStr string
			switch kType {
			case "string":
				valStr, _ = r.client.Get(ctx, k).Result()
			case "hash":
				hLen, _ := r.client.HLen(ctx, k).Result()
				valStr = fmt.Sprintf("(%d fields)", hLen)
			case "list":
				lLen, _ := r.client.LLen(ctx, k).Result()
				valStr = fmt.Sprintf("(%d items)", lLen)
			case "set":
				sCard, _ := r.client.SCard(ctx, k).Result()
				valStr = fmt.Sprintf("(%d members)", sCard)
			case "zset":
				zCard, _ := r.client.ZCard(ctx, k).Result()
				valStr = fmt.Sprintf("(%d members)", zCard)
			default:
				valStr = kType
			}
			sample.Rows = append(sample.Rows, []string{
				truncate(k, 60),
				kType,
				strconv.FormatInt(ttlSec, 10),
				truncate(valStr, 60),
			})
			sample.Raw = append(sample.Raw, []any{k, kType, ttlSec, valStr})
		}
	}

	return sample, nil
}

func (r *redisEngine) Query(ctx context.Context, q string) (*Sample, error) {
	s, _, err := r.RunUserQuery(ctx, q)
	return s, err
}

func (r *redisEngine) RunUserQuery(ctx context.Context, q string) (*Sample, int64, error) {
	q = strings.TrimSpace(q)
	for strings.HasSuffix(q, ";") {
		q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	}
	if q == "" {
		return nil, 0, nil
	}

	upper := strings.ToUpper(q)
	if strings.HasPrefix(upper, "SELECT") {
		tbl := "keys"
		words := strings.Fields(q)
		for i, w := range words {
			if strings.EqualFold(w, "FROM") && i+1 < len(words) {
				tbl = strings.Trim(words[i+1], "\"`[]'")
				break
			}
		}
		s, err := r.PageRows(ctx, QualTable{Name: tbl}, 200, 0)
		return s, -1, err
	}

	args := splitRedisArgs(q)
	if len(args) == 0 {
		return nil, 0, nil
	}
	cmdName := strings.ToUpper(args[0])

	anyArgs := make([]any, len(args))
	for i, a := range args {
		anyArgs[i] = a
	}

	res, err := r.client.Do(ctx, anyArgs...).Result()
	if err != nil {
		return nil, 0, err
	}

	// Write commands report rows affected.
	switch cmdName {
	case "SET", "DEL", "HSET", "HDEL", "LPUSH", "RPUSH", "LPOP", "RPOP", "SADD", "SREM", "ZADD", "ZREM", "EXPIRE":
		var affected int64 = 1
		if n, ok := res.(int64); ok {
			affected = n
		}
		return nil, affected, nil
	}

	sample := &Sample{Total: -1}
	switch v := res.(type) {
	case string:
		sample.Columns = []string{"result"}
		sample.Rows = [][]string{{truncate(v, 60)}}
		sample.Raw = [][]any{{v}}
	case int64:
		sample.Columns = []string{"result"}
		s := strconv.FormatInt(v, 10)
		sample.Rows = [][]string{{s}}
		sample.Raw = [][]any{{v}}
	case []any:
		sample.Columns = []string{"index", "value"}
		for i, item := range v {
			itemStr := fmt.Sprintf("%v", item)
			sample.Rows = append(sample.Rows, []string{strconv.Itoa(i), truncate(itemStr, 60)})
			sample.Raw = append(sample.Raw, []any{int64(i), item})
		}
	case map[any]any:
		sample.Columns = []string{"field", "value"}
		var keys []string
		m := make(map[string]string)
		for mk, mv := range v {
			ks := fmt.Sprintf("%v", mk)
			keys = append(keys, ks)
			m[ks] = fmt.Sprintf("%v", mv)
		}
		sort.Strings(keys)
		for _, ks := range keys {
			sample.Rows = append(sample.Rows, []string{truncate(ks, 60), truncate(m[ks], 60)})
			sample.Raw = append(sample.Raw, []any{ks, m[ks]})
		}
	case map[string]string:
		sample.Columns = []string{"field", "value"}
		var keys []string
		for mk := range v {
			keys = append(keys, mk)
		}
		sort.Strings(keys)
		for _, ks := range keys {
			sample.Rows = append(sample.Rows, []string{truncate(ks, 60), truncate(v[ks], 60)})
			sample.Raw = append(sample.Raw, []any{ks, v[ks]})
		}
	default:
		s := fmt.Sprintf("%v", v)
		sample.Columns = []string{"result"}
		sample.Rows = [][]string{{truncate(s, 60)}}
		sample.Raw = [][]any{{v}}
	}

	return sample, -1, nil
}

func (r *redisEngine) ExecStmt(ctx context.Context, q string) (int64, error) {
	_, n, err := r.RunUserQuery(ctx, q)
	return n, err
}

func (r *redisEngine) UpdateCell(ctx context.Context, q QualTable, pk []string, pkVals []any, col string, val any) (int64, error) {
	if len(pkVals) == 0 {
		return 0, fmt.Errorf("redis update: key required")
	}
	key := fmt.Sprintf("%v", pkVals[0])
	valStr := fmt.Sprintf("%v", val)

	if q.Name == "hashes" {
		field := col
		if len(pkVals) > 1 {
			field = fmt.Sprintf("%v", pkVals[1])
		}
		return r.client.HSet(ctx, key, field, valStr).Result()
	}

	err := r.client.Set(ctx, key, valStr, 0).Err()
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func (r *redisEngine) TableDDL(ctx context.Context, q QualTable) (string, error) {
	var count int64
	if r.client != nil {
		count, _ = r.client.DBSize(ctx).Result()
	}
	return fmt.Sprintf("# Redis Virtual Table: %s\n# Current DB Size: %d keys\n# Commands supported: GET, SET, HGETALL, HSET, DEL, KEYS, SCAN, INFO", q.Name, count), nil
}

func (r *redisEngine) Explain(q string, analyze bool) string {
	args := splitRedisArgs(q)
	if len(args) > 1 && strings.EqualFold(args[0], "GET") {
		return fmt.Sprintf("MEMORY USAGE %s", args[1])
	}
	return fmt.Sprintf("COMMAND INFO %s", q)
}

func splitRedisArgs(s string) []string {
	var args []string
	var b strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := range len(s) {
		c := s[i]
		switch {
		case inQuote:
			if c == quoteChar {
				inQuote = false
				args = append(args, b.String())
				b.Reset()
			} else {
				b.WriteByte(c)
			}
		case c == '"' || c == '\'':
			inQuote = true
			quoteChar = c
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args
}
