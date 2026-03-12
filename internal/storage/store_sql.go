package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// SQLStore implements Store on top of database/sql.
type SQLStore struct {
	dbs    []*sql.DB
	driver string // postgres | mysql | sqlite
	next   atomic.Uint32
	closed atomic.Bool
}

func NewSQLStore(driver string, dsns []string) (*SQLStore, error) {
	if len(dsns) == 0 {
		return nil, errors.New("sql store: empty dsn list")
	}
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		return nil, errors.New("sql store: empty driver")
	}
	driverName := driver
	if driver == "postgres" {
		driverName = "pgx"
	}
	dbs := make([]*sql.DB, 0, len(dsns))
	for _, dsn := range dsns {
		dsn = strings.TrimSpace(dsn)
		if dsn == "" {
			continue
		}
		db, err := sql.Open(driverName, dsn)
		if err != nil {
			for _, c := range dbs {
				_ = c.Close()
			}
			return nil, err
		}
		dbs = append(dbs, db)
	}
	if len(dbs) == 0 {
		return nil, errors.New("sql store: no valid dsn")
	}
	s := &SQLStore{dbs: dbs, driver: driver}
	for _, db := range dbs {
		if err := s.initSchema(db); err != nil {
			for _, c := range dbs {
				_ = c.Close()
			}
			return nil, err
		}
	}
	return s, nil
}

func (s *SQLStore) Record(ev Event) error {
	return s.Log(mapEventKind(ev.Kind), ev)
}

func (s *SQLStore) Log(kind string, data any) error {
	if s == nil {
		return nil
	}
	if s.closed.Load() {
		return sql.ErrConnDone
	}
	if strings.TrimSpace(kind) == "" {
		kind = "events"
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	query := "INSERT INTO logs(ts, kind, data_json) VALUES(?, ?, ?)"
	return s.withDB(func(db *sql.DB) error {
		_, err := db.Exec(s.bind(query), ts, kind, string(raw))
		return err
	})
}

func (s *SQLStore) UpsertPlayer(rec PlayerRecord) error {
	if s == nil {
		return nil
	}
	if s.closed.Load() {
		return sql.ErrConnDone
	}
	if strings.TrimSpace(rec.UUID) == "" {
		return errors.New("player uuid is empty")
	}
	return s.withDB(func(db *sql.DB) error {
		return s.upsertPlayerOn(db, rec)
	})
}

func (s *SQLStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	var lastErr error
	for _, db := range s.dbs {
		if err := db.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (s *SQLStore) Status() string {
	return fmt.Sprintf("sql:%s nodes=%d", s.driver, len(s.dbs))
}

func (s *SQLStore) withDB(fn func(*sql.DB) error) error {
	if len(s.dbs) == 0 {
		return errors.New("sql store: no db")
	}
	start := int(s.next.Add(1))
	var lastErr error
	for i := 0; i < len(s.dbs); i++ {
		db := s.dbs[(start+i)%len(s.dbs)]
		if err := fn(db); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (s *SQLStore) bind(query string) string {
	if s.driver != "postgres" {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteString("$")
			b.WriteString(strconv.Itoa(n))
			n++
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (s *SQLStore) initSchema(db *sql.DB) error {
	createPlayers := "CREATE TABLE IF NOT EXISTS players (uuid TEXT PRIMARY KEY, usid TEXT, name TEXT, ip TEXT, first_seen TEXT, last_seen TEXT, times_joined INTEGER, times_kicked INTEGER, names_json TEXT, ips_json TEXT)"
	createLogs := "CREATE TABLE IF NOT EXISTS logs (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, kind TEXT, data_json TEXT)"
	switch s.driver {
	case "postgres":
		createLogs = "CREATE TABLE IF NOT EXISTS logs (id BIGSERIAL PRIMARY KEY, ts TEXT, kind TEXT, data_json TEXT)"
	case "mysql":
		createLogs = "CREATE TABLE IF NOT EXISTS logs (id BIGINT AUTO_INCREMENT PRIMARY KEY, ts TEXT, kind TEXT, data_json TEXT)"
	}
	if _, err := db.Exec(createPlayers); err != nil {
		return err
	}
	if _, err := db.Exec(createLogs); err != nil {
		return err
	}
	return nil
}

func (s *SQLStore) upsertPlayerOn(db *sql.DB, rec PlayerRecord) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var cur PlayerRecord
	var namesJSON, ipsJSON string
	query := "SELECT usid, name, ip, first_seen, last_seen, times_joined, times_kicked, names_json, ips_json FROM players WHERE uuid = ?"
	row := db.QueryRow(s.bind(query), rec.UUID)
	switch err := row.Scan(&cur.USID, &cur.Name, &cur.IP, &cur.FirstSeen, &cur.LastSeen, &cur.TimesJoined, &cur.TimesKicked, &namesJSON, &ipsJSON); err {
	case nil:
		_ = json.Unmarshal([]byte(namesJSON), &cur.Names)
		_ = json.Unmarshal([]byte(ipsJSON), &cur.IPs)
		if cur.FirstSeen == "" {
			cur.FirstSeen = now
		}
		cur.LastSeen = now
		if rec.USID != "" {
			cur.USID = rec.USID
		}
		if rec.Name != "" {
			cur.Name = rec.Name
			cur.Names = appendUnique(cur.Names, rec.Name)
		}
		if rec.IP != "" {
			cur.IP = rec.IP
			cur.IPs = appendUnique(cur.IPs, rec.IP)
		}
		if rec.TimesJoined > 0 {
			cur.TimesJoined += rec.TimesJoined
		}
		if rec.TimesKicked > 0 {
			cur.TimesKicked += rec.TimesKicked
		}
		namesJSONBytes, _ := json.Marshal(cur.Names)
		ipsJSONBytes, _ := json.Marshal(cur.IPs)
		update := "UPDATE players SET usid=?, name=?, ip=?, first_seen=?, last_seen=?, times_joined=?, times_kicked=?, names_json=?, ips_json=? WHERE uuid=?"
		_, err = db.Exec(s.bind(update), cur.USID, cur.Name, cur.IP, cur.FirstSeen, cur.LastSeen, cur.TimesJoined, cur.TimesKicked, string(namesJSONBytes), string(ipsJSONBytes), rec.UUID)
		return err
	case sql.ErrNoRows:
		cur = PlayerRecord{
			UUID:        rec.UUID,
			USID:        rec.USID,
			Name:        rec.Name,
			IP:          rec.IP,
			FirstSeen:   now,
			LastSeen:    now,
			TimesJoined: rec.TimesJoined,
			TimesKicked: rec.TimesKicked,
		}
		if rec.Name != "" {
			cur.Names = appendUnique(cur.Names, rec.Name)
		}
		if rec.IP != "" {
			cur.IPs = appendUnique(cur.IPs, rec.IP)
		}
		namesJSONBytes, _ := json.Marshal(cur.Names)
		ipsJSONBytes, _ := json.Marshal(cur.IPs)
		insert := "INSERT INTO players(uuid, usid, name, ip, first_seen, last_seen, times_joined, times_kicked, names_json, ips_json) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
		_, err = db.Exec(s.bind(insert), cur.UUID, cur.USID, cur.Name, cur.IP, cur.FirstSeen, cur.LastSeen, cur.TimesJoined, cur.TimesKicked, string(namesJSONBytes), string(ipsJSONBytes))
		return err
	default:
		return err
	}
}
