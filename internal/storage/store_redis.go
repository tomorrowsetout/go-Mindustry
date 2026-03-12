package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client redis.UniversalClient
	kind   string
	closed atomic.Bool
}

func NewRedisStore(kind string, dsn string, cluster bool) (*RedisStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("redis store: empty dsn")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	var client redis.UniversalClient
	if strings.HasPrefix(strings.ToLower(dsn), "redis://") || strings.HasPrefix(strings.ToLower(dsn), "rediss://") {
		opt, err := redis.ParseURL(dsn)
		if err != nil {
			return nil, err
		}
		if cluster {
			client = redis.NewClusterClient(&redis.ClusterOptions{
				Addrs:    []string{opt.Addr},
				Username: opt.Username,
				Password: opt.Password,
			})
		} else {
			client = redis.NewClient(opt)
		}
	} else {
		addrs := splitCSV(dsn)
		if len(addrs) == 0 {
			return nil, errors.New("redis store: invalid address list")
		}
		if cluster {
			client = redis.NewClusterClient(&redis.ClusterOptions{Addrs: addrs})
		} else {
			client = redis.NewClient(&redis.Options{Addr: addrs[0]})
		}
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &RedisStore{client: client, kind: kind}, nil
}

func (s *RedisStore) Record(ev Event) error {
	return s.Log(mapEventKind(ev.Kind), ev)
}

func (s *RedisStore) Log(kind string, data any) error {
	if s == nil || s.client == nil {
		return nil
	}
	if s.closed.Load() {
		return redis.ErrClosed
	}
	if strings.TrimSpace(kind) == "" {
		kind = "events"
	}
	raw, err := json.Marshal(LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      kind,
		Data:      data,
	})
	if err != nil {
		return err
	}
	key := "logs:" + kind
	return s.client.LPush(context.Background(), key, raw).Err()
}

func (s *RedisStore) UpsertPlayer(rec PlayerRecord) error {
	if s == nil || s.client == nil {
		return nil
	}
	if s.closed.Load() {
		return redis.ErrClosed
	}
	if strings.TrimSpace(rec.UUID) == "" {
		return errors.New("player uuid is empty")
	}
	key := "player:" + rec.UUID
	ctx := context.Background()
	cur, _ := s.client.HGetAll(ctx, key).Result()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	names := splitCSV(cur["names"])
	ips := splitCSV(cur["ips"])
	if rec.Name != "" {
		names = appendUnique(names, rec.Name)
	}
	if rec.IP != "" {
		ips = appendUnique(ips, rec.IP)
	}
	tj := parseIntSafe(cur["times_joined"])
	tk := parseIntSafe(cur["times_kicked"])
	tj += int64(rec.TimesJoined)
	tk += int64(rec.TimesKicked)
	firstSeen := cur["first_seen"]
	if firstSeen == "" {
		firstSeen = now
	}
	fields := map[string]any{
		"uuid":         rec.UUID,
		"usid":         pickNonEmpty(rec.USID, cur["usid"]),
		"name":         pickNonEmpty(rec.Name, cur["name"]),
		"ip":           pickNonEmpty(rec.IP, cur["ip"]),
		"first_seen":   firstSeen,
		"last_seen":    now,
		"times_joined": tj,
		"times_kicked": tk,
		"names":        strings.Join(names, ","),
		"ips":          strings.Join(ips, ","),
	}
	return s.client.HSet(ctx, key, fields).Err()
}

func (s *RedisStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

func (s *RedisStore) Status() string {
	return "redis:" + s.kind
}

func pickNonEmpty(next, fallback string) string {
	if strings.TrimSpace(next) != "" {
		return next
	}
	return fallback
}
